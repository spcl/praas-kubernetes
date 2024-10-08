import asyncio
import sys

import time
import json
import logging
import multiprocessing
import os
import shutil
import tempfile
from threading import Semaphore, Event, Lock
from typing import Dict

import psutil as psutil
import traceback

import wget
from fastapi import APIRouter, WebSocket, WebSocketDisconnect

import praassdk.types
import praassdk.protocol

from . import msg as praas_messaging, metrics, state
from .ws import WebsocketBase
from .config import AppConfig, get_logger
from .workqueue import WorkQueue
from .runner import func_runner, FunctionExecutionError
from .types import Invocation, InvocationTask, IOAdaptor, ResourceManager

router = APIRouter()

manager = ResourceManager(state.cpu_count)

func_to_wheel = dict()
func_state_lock = Lock()
func_events: Dict[str, Event] = dict()
func_locks: Dict[str, Semaphore] = dict()
func_paths: Dict[str, str] = dict()
pool = multiprocessing.Pool(processes=state.cpu_count)


async def handle_invocation(work_item: InvocationTask):
    inv, websocket, lock = work_item
    try:
        run_task = asyncio.create_task(run_invocation(work_item))
        with lock:
            await websocket.send_json({"success": True, "func-id": inv.id, "operation": "invocation"})
        was_ok, result, stderr = await run_task

        if was_ok:
            if hasattr(result, "__dict__"):
                result = result.__dict__
            data = {"success": True, "func-id": inv.id, "operation": "run", "result": result}
        else:
            data = {"success": False, "func-id": inv.id, "reason": "Failed to run function to completion",
                    "output": stderr}
    except:
        logging.getLogger().error(traceback.format_exc())
        data = {"success": False, "reason": "Internal Server Error"}

    # Send results back to user
    with lock:
        await websocket.send_json(data)

    # De-allocate resources
    metrics.metrics_state.end_func(inv.id)
    manager.free(inv.cores, inv.get_mem_req())


def can_run(inv: InvocationTask) -> bool:
    # Check if we have enough memory
    inv = inv[0]
    mem_req = inv.get_mem_req()
    data_plane_usage = psutil.Process().memory_info().rss
    guaranteed_available = state.mem_bytes-data_plane_usage
    success, cores = manager.reserve(inv.cpu, mem_req, guaranteed_available)
    if not success:
        return False
    inv.set_cores(cores)
    return True


def func_added(inv: InvocationTask) -> None:
    inv, _, _ = inv
    metrics.metrics_state.start_func(inv.id)

    if inv.function not in func_locks:
        func_locks[inv.function] = Semaphore(1)
    with func_state_lock:
        if inv.function not in func_events:
            func_events[inv.function] = Event()

    for proc in inv.deps.values():
        connection_handler = praas_messaging.establish_connection(proc)
        if connection_handler is not None:
            asyncio.create_task(connection_handler)


state.work_queue = WorkQueue(handler=handle_invocation, req=can_run, new_notification=func_added)


@router.websocket("/data-plane")
async def data_plane(websocket: WebSocket):
    logger = get_logger()
    await websocket.accept()
    lock = Lock()
    try:
        is_connected = True
        while is_connected:
            req = await websocket.receive_text()
            try:
                if not metrics.metrics_state.can_accept:
                    with lock:
                        await websocket.send_json(
                            {"success": False, "reason": "Cannot accept function invocations anymore"})
                else:
                    req = json.loads(req)
                    req_repr = {x: req[x] for x in req if x != "args"}
                    logger.debug(f"Received new func invocation request: {req_repr}")
                    inv = Invocation(**req)
                    with lock:
                        await websocket.send_json({"success": True, "operation": "scheduled", "func-id": inv.id})
                    state.work_queue.add_task((inv, websocket, lock))
            except Exception as e:
                logger.exception("Failed to accept invocation")
                with lock:
                    await websocket.send_json({"success": False, "reason": repr(e)})
    except WebSocketDisconnect as e:
        logger.debug(f"Client disconnected with code: {e.code}")


async def run_invocation(task: InvocationTask):
    invocation, ws, lock = task
    logger = get_logger()
    logger.debug(f"Run invocation: {invocation.id}-{invocation.function}")
    start = time.perf_counter()
    config = AppConfig()
    recv_timeout = config.recv_timeout

    # Get the function image if we don't have it yet
    # TODO(gr): Look for updates to the image? (versioning)
    semaphore = func_locks[invocation.function]
    if invocation.function not in func_paths and semaphore.acquire(blocking=False):
        func_dir = os.path.join(config.function_dir, invocation.function)
        wheel, funcs = download_func(invocation.function, config)
        with func_state_lock:
            for func in funcs:
                func_to_wheel[func] = wheel
                func_paths[func] = func_dir
                if func in func_events:
                    event = func_events[func]
                else:
                    event = Event()
                    func_events[func] = event
                event.set()

    else:
        with func_state_lock:
            event = func_events[invocation.function]
        event.wait()
        wheel = func_to_wheel[invocation.function]
        func_dir = func_paths[invocation.function]

    # Run the function in a subprocess
    runtime_pipe, func_pipe = multiprocessing.Pipe()
    runner_args = (
        invocation.id,
        func_dir,
        wheel,
        invocation.function,
        invocation.args,
        invocation.cores,
        invocation.get_mem_req(),
        func_pipe,
        config.recv_timeout
    )
    func_task = pool.apply_async(func_runner, runner_args)
    logger.debug(f"Function {invocation.function} is running")

    buffer = IOAdaptor(runtime_pipe)
    errors_during_run = list()
    task_finished = False
    while not task_finished:
        try:
            # Read operation flag
            flag = praassdk.protocol.__read_flag(buffer)
        except EOFError:
            break

        # Perform operation according to flag
        if flag == praassdk.types.MsgType.RETVAL:
            break
        elif flag == praassdk.types.MsgType.PUT:
            # Get the message
            target, name, msg_bytes = get_message(buffer)
            try:
                # Send the message
                if target == invocation.id:
                    process = "local"
                else:
                    if target not in invocation.deps:
                        raise Exception(
                            f"Function {target} missing from dependency map of function "
                            f"{invocation.id}.{invocation.function}"
                        )
                    process = invocation.deps[target]
                await praas_messaging.send_msg(invocation.id, target, name, process, msg_bytes)
            except Exception:
                errors_during_run.append(traceback.format_exc())

        elif flag == praassdk.types.MsgType.BRDCAST:
            targets, name, msg_bytes = get_broadcast(buffer)
            try:
                funcs_map = build_func_map(targets, invocation)
                await praas_messaging.broadcast(invocation.id, name, funcs_map, msg_bytes)
            except Exception:
                errors_during_run.append(traceback.format_exc())

        elif flag == praassdk.types.MsgType.GET or flag == praassdk.types.MsgType.DEL:
            # Get the source
            src, target, name = handle_msg_read(buffer)
            try:
                if target == -1:
                    target = invocation.id
                    process = "local"
                else:
                    if target not in invocation.deps:
                        raise Exception(
                            f"Function {target} missing from dependency map of function "
                            f"{invocation.id}.{invocation.function}"
                        )
                    process = invocation.deps[target]
                # Get the message
                delete = flag == praassdk.types.MsgType.DEL
                msg = await asyncio.wait_for(
                    praas_messaging.receive_msg(invocation.id, src, target, name, process, delete),
                    recv_timeout
                )
                # Transmit message to function
                handle_msg_reply(buffer, msg)
            except Exception:
                handle_error(buffer, traceback.format_exc())

        elif flag == praassdk.types.MsgType.INVOKE:
            # Get request obj
            req = read_invoke(buffer)
            # Handle the request
            await handle_func_invoke(invocation, req, ws, lock)

        elif flag == -1:
            # Probably the subprocess exited
            break
        else:
            error_msg = "Unknown flag value: " + str(flag)
            logger.error(error_msg)
            raise Exception(error_msg)

    # Function stopped executing, cleanup
    func_pipe.close()
    runtime_pipe.close()

    # Get outcome of execution
    stderr = ""
    result = None
    success = False
    if len(errors_during_run) == 0:
        try:
            result = func_task.get()
            success = True
        except FunctionExecutionError as e:
            stderr = str(e)
    else:
        stderr = " and ".join(errors_during_run)

    end = time.perf_counter()
    run_time = end - start
    logger.debug(f"End invocation: {invocation.id}-{invocation.function} (time: {run_time * 1000} ms)")
    return success, result, stderr


def get_message(buffer: praassdk.ProcessChannel):
    target = praassdk.protocol.__read_number(buffer)
    name = praassdk.protocol.__read_obj(buffer)
    msg = praassdk.protocol.__read_obj(buffer, raw=True)

    return target, name, msg


def get_broadcast(buffer: praassdk.ProcessChannel):
    targets = praassdk.protocol.__read_obj(buffer)
    name = praassdk.protocol.__read_obj(buffer)
    msg = praassdk.protocol.__read_obj(buffer, raw=True)

    return targets, name, msg


def build_func_map(targets, invocation):
    func_map = dict()
    for target in targets:
        if target == invocation.id:
            continue

        if target not in invocation.deps:
            raise Exception(
                f"Function {target} missing from dependency map of function "
                f"{invocation.id}.{invocation.function}"
            )
        process = invocation.deps[target]
        if process not in func_map:
            func_map[process] = list()
        func_map[process].append(target)
    return func_map


def handle_msg_read(buffer: praassdk.ProcessChannel):
    source = praassdk.protocol.__read_number(buffer)
    target = praassdk.protocol.__read_number(buffer)
    name = praassdk.protocol.__read_obj(buffer)
    return source, target, name


def handle_msg_reply(buffer: praassdk.ProcessChannel, msg):
    praassdk.protocol.__write_flag(praassdk.types.MsgType.REP, buffer)
    praassdk.protocol.__write_obj(msg, buffer, raw=True)
    buffer.flush()


def handle_error(buffer: praassdk.ProcessChannel, error_msg: str):
    praassdk.protocol.__write_flag(praassdk.types.MsgType.ERR, buffer)
    praassdk.protocol.__write_obj(error_msg, buffer)
    buffer.flush()


def read_invoke(buffer: praassdk.ProcessChannel):
    req = praassdk.protocol.__read_obj(buffer)
    return req


async def handle_func_invoke(parent: Invocation, req: praassdk.types.InvokeRequest, ws: WebsocketBase, lock: Lock):
    inv = Invocation(id=req.id, function=req.func, args=req.args, cpu=req.cpu, mem=req.mem)
    inv.deps = parent.deps

    if req.process.lower() == "local" or req.process == praas_messaging.self_process_id:
        state.work_queue.add_task((inv, ws, lock))
    else:
        await praas_messaging.invoke_function(req.process, inv)


def download_func(name: str, app_config: AppConfig):
    start = time.perf_counter()

    # Setup temporary workplace
    temp_dir = tempfile.TemporaryDirectory()
    filepath = os.path.join(temp_dir.name, "temp_file.zip")
    func_path = os.path.join(app_config.function_dir, name)
    os.makedirs(func_path, exist_ok=True)

    # Download and extract the function
    url = app_config.function_store + "/function-image/" + name
    wget.download(url, filepath, None)
    shutil.unpack_archive(filepath, func_path, "zip")
    temp_dir.cleanup()

    entry_path = os.path.join(func_path, "entrypoint")
    with open(entry_path, "r") as f:
        entrypoint = f.read()
    funcs_path = os.path.join(func_path, "functions")
    with open(funcs_path, "r") as f:
        funcs = f.readlines()
        funcs = map(lambda func: func[:-1], funcs)

    end = time.perf_counter()
    duration = end - start
    logger = get_logger()
    logger.debug(f"Download time for {name} was: {duration *1000} ms")
    return entrypoint, funcs


def format_exception(e: Exception):
    return str(e)


# Abandoned: Function for downloading the image and reading the header containing all the names
# def download_img(url: str, out: str):
#     response = requests.get(url, stream=True)
#     if not response.ok:
#         raise Exception(f"Could not download function image: {response.status_code} ({response.reason})")
#
#     functions = response.headers["X-PRAAS-FUNCS"]
#
#     with open(out, "wb") as out_file:
#         response.raw.decode_content = True
#         shutil.copyfileobj(response.raw, out_file)
#
#     return functions.split(";")
