import copy

import json

import time

import asyncio
from concurrent.futures import ThreadPoolExecutor

import os
import requests
from threading import Condition, Event, Lock

from typing import Dict, Iterable, Set, List

import websockets
from fastapi import APIRouter, WebSocket
from starlette.websockets import WebSocketDisconnect

from . import state, config
from .types import Invocation
from .config import AppConfig
from .ws import WebsocketChannel, WebsocketBase, NullWebsocket

router = APIRouter()

app: str = os.getenv("APP_ID")
self_process_id: str = os.getenv("PROCESS_ID")
local_funcs = set()
peer_handlers = set()
outgoing_msgs = dict()
func_locations = dict()
local_msg_store = dict()
local_msg_conditions = set()
local_recv_store = dict()
local_recv_conditions = set()
peers: Dict[str, WebsocketBase] = dict()
peer_locks: Dict[str, Lock] = dict()
peer_conn_events: Dict[str, Event] = dict()

pool = ThreadPoolExecutor()


@router.websocket("/")
async def coordinator(websocket: WebSocket):
    logger = config.get_logger()
    await websocket.accept()
    other_process = await websocket.receive_text()
    sorted_ids = sorted((self_process_id, other_process))
    if other_process not in peers or sorted_ids[0] == self_process_id:
        logger.debug(f"New connection with: {other_process}")
        lock = Lock()
        peers[other_process] = websocket
        peer_locks[other_process] = lock
        await websocket.send_text("accept")
        await __handle_peer(websocket, other_process, lock)
    else:
        logger.debug(f"Already have connection to {other_process}")
        await websocket.send_text("reject")


async def initiate_connection(ip: str, process: str, stable=True) -> WebsocketBase:
    logger = config.get_logger()
    url = f"ws://{ip}:8080/coordinate/"
    logger.debug(f"Dial: {url}")

    start = time.perf_counter()
    retry = 20
    ws = None
    while ws is None:
        try:
            ws = await websockets.connect(url)
        except:
            retry -= 1
            if retry <= 0:
                raise
            await asyncio.sleep(0.1)

    ws_wrapper = WebsocketChannel(ws)
    await ws_wrapper.send_text(self_process_id)
    response = await ws_wrapper.receive_text()

    duration = time.perf_counter() - start
    if stable and response == "accept":
        __add_peer(ws_wrapper, process)
    logger.debug(f"Connection was established in {duration*1000} ms")
    if response == "reject":
        return None

    return ws_wrapper


def __msg_in_store(source, receiver, name, store):
    if receiver not in store:
        return False
    if source not in store[receiver]:
        return False
    return name in store[receiver][source]


async def receive_msg(fid: int, source: int, receiver_id: int, name: str, process: str, delete: bool) -> bytes:
    logger = config.get_logger()
    # logger.debug(f"receive_msg {source}->{receiver_id}:{name}")
    # Wait until a message is available in the local box
    location, ws, lock = await __get_process_channel(process)

    if location == "local":
        return await __receive_local(source, receiver_id, name, delete, local_msg_store, local_msg_conditions)
    else:
        # logger.debug(f"Remote get msg: ({source})->({receiver_id}):{name}@{process}")
        msg_bytes = await __receive_remote(fid, source, receiver_id, name, delete, ws, lock)
        if process not in peers:
            logger.debug("Close connection after get")
            await ws.close()
        return msg_bytes


async def __receive_remote(fid: int, source: int, receiver_id: int, name: str, delete: bool, ws: WebsocketBase, lock) -> bytes:
    with lock:
        await ws.send_json({
            "operation": "recv_msg",
            "id": fid,
            "source": source,
            "target": receiver_id,
            "name": name,
            "delete": delete
        })
    if fid not in local_recv_store:
        local_recv_store[fid] = dict()
    msg_bytes = await __receive_local(source, receiver_id, name, True, local_recv_store[fid], local_recv_conditions)
    return msg_bytes


def __wait_for_msg(source: int, target: int, name: str, cond, store: dict):
    cond.acquire()
    cond.wait_for(lambda: __msg_in_store(source, target, name, store))
    cond.release()


async def __receive_local(source: int, receiver_id: int, name: str, delete: bool, store: dict, conditions: Set[Condition]) -> bytes:
    my_cond = Condition()
    conditions.add(my_cond)
    loop = asyncio.get_running_loop()
    await loop.run_in_executor(None, __wait_for_msg, source, receiver_id, name, my_cond, store)
    conditions.remove(my_cond)

    # Get (+ remove) first item
    logger = config.get_logger()
    try:
        msg = store[receiver_id][source][name]
    except:
        logger.exception(f"Waited for msg ({source})->({receiver_id}):{name} ({local_msg_store})")
        raise

    if delete:
        del store[receiver_id][source][name]
    return msg


def establish_connection(process_id: str, stable=True):
    logger = config.get_logger()
    if process_id in peer_conn_events or process_id in peers or process_id == self_process_id:
        return
    logger.debug(f"connection request {process_id} (stable={stable})")

    peer_conn_events[process_id] = Event()
    ip = __get_ip(process_id)
    return initiate_connection(ip, process_id, stable)


async def __get_process_channel(process_id: str) -> (str, WebsocketBase):
    # Is it local
    if process_id == self_process_id or process_id == "local":
        return "local", None, None

    # It's not local so it must be remote
    if process_id in peers:
        ws = peers[process_id]
        lock = peer_locks[process_id]
    elif process_id in peer_conn_events:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, peer_conn_events[process_id].wait)

        ws = peers[process_id]
        lock = peer_locks[process_id]
    else:
        # There is no connection, so make one
        ws = await establish_connection(process_id, stable=False)
        lock = Lock()

    return "remote", ws, lock


def __get_ip(process_id: str):
    config = AppConfig()
    url = config.process_nameservice + "/process"
    url += f"?app={app}&pid={process_id}"
    response = requests.get(url).json()
    return response["data"]["ip"]


async def broadcast(source: int, name: str, targets: dict, msg_bytes: bytes):
    logger = config.get_logger()
    # logger.debug(f"broadcast {source}->{targets}:{name}")
    for process, functions in targets.items():
        location, ws, lock = await __get_process_channel(process)

        # It is a local message, save it in the local box
        if location == "local":
            for function in functions:
                __store_msg(local_msg_store, source, function, name, msg_bytes)
            __signal_change(local_msg_conditions)

        # Remote message, send it to the process that has this function
        else:
            await __remote_broadcast(ws, source, functions, name, msg_bytes, lock)
            if process not in peers:
                await ws.close()


async def send_msg(source: int, target: int, name: str, process_id: str, msg: bytes):
    logger = config.get_logger()
    location, ws, lock = await __get_process_channel(process_id)

    # It is a local message, save it in the local box
    if location == "local":
        __store_msg(local_msg_store, source, target, name, msg)
        __signal_change(local_msg_conditions)

    # Remote message, send it to the process that has this function
    else:
        # logger.debug(f"Remote send message ({source})->({target}):{name}@{process_id}")
        await __remote_send_msg(ws, source, target, name, msg, lock)
        if process_id not in peers:
            await ws.close()


def __store_msg(store: dict, source: int, target: int, name: str, msg: bytes):
    logger = config.get_logger()
    # logger.debug(f"__store_msg {source}->{target}:{name}")
    if target not in store:
        store[target] = dict()
    if source not in store[target]:
        store[target][source] = dict()
    store[target][source][name] = msg


async def __remote_send_msg(ws: WebsocketBase, source: int, target: int, name: str, msg: bytes, lock: Lock):
    with lock:
        await ws.send_json({"operation": "send_msg", "source": source, "target": target, "name": name})
        await ws.send_bytes(msg)


async def __remote_broadcast(ws: WebsocketBase, source: int, targets: List[int], name: str, msg: bytes, lock: Lock):
    logger = config.get_logger()
    # logger.debug(f"__remote_broadcast {source}->{targets}:{name}")
    with lock:
        await ws.send_json({
            "operation": "broadcast",
            "source": source,
            "targets": targets,
            "name": name,
        })
        await ws.send_bytes(msg)


async def notify_end_proc():
    await __broadcast_msg({"operation": "proc_end"})
    try:
        for ws in peers.values():
            await ws.close()
    except:
        pass


async def __broadcast_msg(msg: dict):
    for peer_id in list(peers):
        try:
            peer = peers[peer_id]
            await peer.send_json(msg)
        except:
            await __recv_proc_end(peer_id)


async def __recv_msg(ws: WebsocketBase, source: int, target: int, name: str):
    msg_bytes = await ws.receive_bytes()
    __store_msg(local_msg_store, source, target, name, msg_bytes)
    __signal_change(local_msg_conditions)


async def __recv_broadcast(ws: WebsocketBase, source: int, targets: List[int], name: str):
    logger = config.get_logger()
    # logger.debug(f"__recv_broadcast {source}->{targets}:{name}")
    msg_bytes = await ws.receive_bytes()
    for target in targets:
        __store_msg(local_msg_store, source, target, name, msg_bytes)
    __signal_change(local_msg_conditions)


async def __recv_proc_end(peer_id: str):
    try:
        await peers[peer_id].close()
    except:
        pass
    try:
        del peers[peer_id]
    except:
        pass
    # TODO(gr): Delete IP from known IPs
    # TODO(gr): Do we need to do anything about messages we might have or will have?


def __add_peer(ws: WebsocketBase, process_id: str):
    logger = config.get_logger()
    logger.debug("__add_peer")
    peers[process_id] = ws
    lock = Lock()
    peer_locks[process_id] = lock
    if process_id in peer_conn_events:
        peer_conn_events[process_id].set()
    asyncio.create_task(__handle_peer(ws, process_id, lock))
    logger.debug("__add_peer finished")


def __parse_req(msg):
    pos = 0
    reqs = list()
    decoder = json.JSONDecoder()
    while pos < len(msg):
        obj, pos = decoder.raw_decode(msg, pos)
        reqs.append(obj)
    return reqs


async def __handle_peer(ws: WebsocketBase, peer_id: str, lock: Lock):
    logger = config.get_logger()
    logger.debug("Start listening to peer: " + peer_id)
    connection_open = True
    loop = asyncio.get_running_loop()
    background_tasks = set()
    while connection_open:
        try:
            reqs = await ws.receive_text()
            reqs = __parse_req(reqs)
            for req in reqs:
                op = req["operation"]
                if op == "proc_end":
                    await __recv_proc_end(peer_id)
                    connection_open = False
                elif op == "func_inv":
                    await __recv_func_inv(req)
                elif op == "send_msg":
                    await __recv_msg(ws, req["source"], req["target"], req["name"])
                elif op == "broadcast":
                    await __recv_broadcast(ws, req["source"], req["targets"], req["name"])
                elif op == "recv_msg":
                    requester = req["id"]
                    src = req["source"]
                    target = req["target"]
                    name = req["name"]
                    delete = req["delete"]
                    task = loop.create_task(__handle_msg_send(ws, requester, src, target, name, delete, lock))
                    background_tasks.add(task)
                    task.add_done_callback(background_tasks.discard)
                elif op == "remote_recv_reply":
                    await __recv_reply(ws, req["requester"], req["source"], req["target"], req["name"])
                else:
                    raise Exception(f"Received invalid coordination message: {reqs}")
        except (WebSocketDisconnect, websockets.ConnectionClosed) as disc:
            logger.debug(f"Peer: {peer_id} closed connection, code: {disc}")
            connection_open = False
            if peer_id in peers and peers[peer_id] == ws:
                del peers[peer_id]
        except json.JSONDecodeError as e:
            logger.exception(f"Json decode issue with doc: '{e.doc}'")
            connection_open = False
            if peer_id in peers and peers[peer_id] == ws:
                del peers[peer_id]
        except:
            logger.exception(f"Unexpected exception during handling peer: {peer_id}")
            connection_open = False
            if peer_id in peers and peers[peer_id] == ws:
                del peers[peer_id]

    logger.debug("Peer handler finished: " + peer_id)


async def __recv_reply(ws, requester, src, target, name):
    msg_bytes = await ws.receive_bytes()
    __store_msg(local_recv_store[requester], src, target, name, msg_bytes)
    __signal_change(local_recv_conditions)


async def __remote_recv_reply(ws, requester, src, target, name, msg_bytes, lock):
    with lock:
        await ws.send_json({
            "operation": "remote_recv_reply",
            "requester": requester,
            "source": src,
            "target": target,
            "name": name
        })
        await ws.send_bytes(msg_bytes)


async def __handle_msg_send(ws, requester, src, target, name, delete, lock):
    msg_bytes = await __receive_local(src, target, name, delete, local_msg_store, local_msg_conditions)
    await __remote_recv_reply(ws, requester, src, target, name, msg_bytes, lock)


async def __recv_func_inv(req: dict):
    inv = Invocation(**req)
    state.work_queue.add_task((inv, NullWebsocket()))


async def invoke_function(process: str, invocation: Invocation):
    # TODO(gr): handle processes that don't exist (anymore or yet)
    # Get channel
    if process not in peers:
        ws = await establish_connection(process, stable=False)
    else:
        ws = peers[process]
    inv_req = invocation.dict()
    inv_req["operation"] = "func_inv"

    await ws.send_json(inv_req)
    if process not in peers:
        await ws.close()


def __signal_change(conditions: Iterable[Condition]):
    conds = copy.copy(conditions)
    for cond in conds:
        cond.acquire()
        cond.notify()
        cond.release()
