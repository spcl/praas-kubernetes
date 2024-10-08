import asyncio
import traceback
from asyncio import Event, Queue

import time
import json
import sys

import aiohttp
from typing import Any, List, Dict, Iterable, Set, Collection

import websockets
from websockets import exceptions

from executor.config import AppConfig

import requests


def __post_to_praas(url, body):
    resp = requests.post(url, body)
    if resp.status_code != 200:
        raise Exception("Could not contact praas control plane")
    body = resp.json()
    if not body["success"]:
        raise Exception("Request did not succeed")
    return body["data"]


class DataPlaneConnectionPool:
    connections: Dict[str, websockets.WebSocketClientProtocol]
    process_map: Dict[Any, str]
    app_id: int
    config: AppConfig
    needs_conn: Set[Any]
    events: Dict[Any, Event]
    handler_tasks: set
    messages: Queue
    listening: bool
    func_map: Dict[int, str]

    def __init__(self, cfg: AppConfig):
        self.events = dict()
        self.needs_conn = set()
        self.handler_tasks = set()
        self.messages = Queue()
        self.func_map = dict()
        self.connections = dict()
        self.process_map = dict()
        self.config = cfg
        self.app_id = -1
        self.listening = False
        self.cleanup = False

    def schedule(self, processes: Collection[Any], cleanup):
        # Do we have to create new praas processes:
        current_connections = list(self.connections.keys())
        conn_idx = 0
        conn_len = len(self.connections)
        self.cleanup = cleanup

        for p in processes:
            event = Event()
            self.events[p] = event
            if conn_idx < conn_len:
                praas_proc = current_connections[conn_idx]
                conn_idx += 1
                self.process_map[p] = praas_proc
                event.set()
            else:
                self.needs_conn.add(p)
        return self

    async def __aenter__(self):
        self.listening = True
        async with aiohttp.ClientSession() as session:
            # Make sure we have an app
            if self.app_id == -1:
                self.app_id = await self.__create_app(session)

            # Ensure each given reference has a praas process mapped to it
            new_procs = await self.__ensure_procs(session)

        # Map the newly created praas processes
        tasks = set()
        start = time.perf_counter()
        for idx, p_ref in enumerate(self.needs_conn):
            pod_data = new_procs[idx]
            self.process_map[p_ref] = pod_data["pid"]
            task = asyncio.create_task(self.__create_connection(p_ref, pod_data))
            tasks.add(task)
        await asyncio.gather(*tasks)
        duration = time.perf_counter() - start
        print(f"Connection time: {duration * 1000}")
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        self.listening = False
        self.needs_conn.clear()
        self.events.clear()
        self.func_map.clear()

        # Empty message queues
        for _ in range(self.messages.qsize()):
            self.messages.get_nowait()
            self.messages.task_done()

        if self.cleanup:
            async with aiohttp.ClientSession() as session:
                await self.__delete_app(session)

    async def __post_to_praas(self, session: aiohttp.ClientSession, endpoint: str, body):
        url = self.config.praas_url + endpoint
        async with session.post(url, data=body) as resp:
            if not resp.ok:
                raise Exception("Could not contact praas control plane")

            text_data = await resp.text()
            data = json.loads(text_data)
            if not data["success"]:
                raise Exception("Request did not succeed, because: " + data["reason"])
            print("data is:", data, file=sys.stderr, flush=True)
            return data["data"]

    async def __create_app(self, session: aiohttp.ClientSession):
        start = time.perf_counter()
        data = await self.__post_to_praas(session, "/application", "{}")
        end = time.perf_counter()
        duration = end - start
        print(f"App creation time: {duration * 1000}")
        return data["app-id"]

    async def __delete_app(self, session):
        url = self.config.praas_url + "/application"
        body = {"app-id": self.app_id}
        await session.delete(url, json=body)
        self.app_id = -1

    async def __ensure_procs(self, session):
        if len(self.needs_conn) == 0:
            return []
        print("Create connections:", self.needs_conn, file=sys.stderr, flush=True)
        start = time.perf_counter()
        processes = ",".join(["{}"] * len(self.needs_conn))
        body = f'{{"app-id":{self.app_id},"processes":[{processes}]}}'
        data = await self.__post_to_praas(session, "/process", body)
        duration = time.perf_counter() - start
        print(f"Pod creation time: {duration * 1000}")
        return data["processes"]

    async def __create_connection(self, ref: Any, pod_data: dict):
        addr = f"ws://{pod_data['ip']}:8080/data-plane"
        ws = None
        while ws is None:
            try:
                ws = await websockets.connect(addr)
            except websockets.exceptions.InvalidStatusCode as e:
                if e.status_code == 500:
                    time.sleep(0.1)
                else:
                    raise
            except ConnectionRefusedError:
                time.sleep(0.1)

        self.connections[pod_data["pid"]] = ws
        connection_handler_task = asyncio.create_task(self.__handle_connection(pod_data["pid"]))
        self.handler_tasks.add(connection_handler_task)
        connection_handler_task.add_done_callback(self.handler_tasks.discard)
        self.events[ref].set()

    async def submit(self, process: Any, fid: int, function: str, args: List[str], deps: Dict[int, int], mem: int,
                     cpu: int):
        await self.events[process].wait()
        praas_process = self.process_map[process]
        self.func_map[fid] = praas_process
        proper_deps = self.__remap_deps(deps)
        req = {
            "id": fid,
            "function": function,
            "args": args,
            "deps": proper_deps,
            "mem": f"{mem}B",
            "cpu": cpu
        }
        req_str = json.dumps(req)
        ws = self.connections[praas_process]
        await ws.send(req_str)

    async def get_outputs(self, functions: Collection[int]):
        count = len(functions)
        finished_count = 0
        results = dict()
        while finished_count < count:
            msg = await asyncio.wait_for(self.messages.get(), 1800)
            if not msg["success"]:
                error_str = ""
                if "reason" in msg:
                    error_str = msg["reason"]
                if "output" in msg:
                    error_str = msg["output"]
                print("Erroneous message:", msg, file=sys.stderr, flush=True)
                raise Exception("There was an error while executing functions: " + error_str)

            if "operation" in msg:
                op = msg["operation"]
                if op == "closed":
                    # Connection was closed, is it a problem?
                    process = msg["proc"]
                    for f in functions:
                        if self.func_map[f] == process:
                            if f not in results:
                                raise Exception("We lost the process running: " + str(f))

                elif op == "run" and "func-id" in msg:
                    fid = msg["func-id"]
                    if fid in functions:
                        finished_count += 1
                        results[fid] = msg["result"]

        return results

    def __remap_deps(self, deps: Dict[int, int]):
        new_deps = dict()
        for fid, pid in deps.items():
            new_deps[fid] = self.process_map[pid]
        return new_deps

    async def __handle_connection(self, pid: str):
        ws = self.connections[pid]
        try:
            decoder = json.JSONDecoder()
            while True:
                msg = await ws.recv()

                if not self.listening:
                    continue

                pos = 0
                while pos < len(msg):
                    obj, pos = decoder.raw_decode(msg, pos)
                    obj["pid"] = pid
                    await self.messages.put(obj)
        except websockets.exceptions.ConnectionClosedError:
            await ws.close()
        except:
            traceback.print_exc(file=sys.stderr)
            sys.stderr.flush()
            await ws.close()
        finally:
            print("Connection to", pid, "was lost", file=sys.stderr, flush=True)
            del self.connections[pid]
