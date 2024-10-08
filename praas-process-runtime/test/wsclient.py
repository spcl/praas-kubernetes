import asyncio
import json
import pickle
import sys

import time

import websockets


async def __close_ws(ws, func_id):
    msg = json.dumps({"operation": "func_end", "func_id": func_id})
    await ws.send(msg)
    time.sleep(10)
    print("Going to close ws now")
    await ws.send('{"operation": "proc_end"}')
    await ws.close()


async def put():
    print("Connect")
    ws = await websockets.connect("ws://127.0.0.1:8002/coordinate/")
    msg = {"operation": "func_run", "func_id": 1}
    msg = json.dumps(msg)
    print("Send:", msg)
    await ws.send(msg)
    time.sleep(1)
    msg = json.dumps({"operation": "send_msg", "source": 1, "target": 2})
    await ws.send(msg)
    msg = pickle.dumps("Hello World from test")
    await ws.send(msg)
    time.sleep(2)
    await __close_ws(ws, 1)


async def get():
    print("Connect")
    ws = await websockets.connect("ws://127.0.0.1:8002/coordinate/")
    msg = {"operation": "func_run", "func_id": 2}
    msg = json.dumps(msg)
    print("Send:", msg)
    await ws.send(msg)
    time.sleep(1)
    obj_is_next = False
    while not obj_is_next:
        msg = await ws.recv()
        print("Received:", msg)
        req = json.loads(msg)
        obj_is_next = req["operation"] == "send_msg"
    msg_bytes = await ws.recv()
    msg_obj = pickle.loads(msg_bytes)
    print("Received msg object:", msg_obj)
    time.sleep(2)
    await __close_ws(ws, 2)


thing_to_run = None
if sys.argv[1] == "put":
    thing_to_run = put()
elif sys.argv[1] == "get":
    thing_to_run = get()
else:
    print("Unknown scenario:", sys.argv[1], file=sys.stderr)
    exit(1)

asyncio.run(thing_to_run)

