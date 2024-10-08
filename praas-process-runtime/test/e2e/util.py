import asyncio
import json
import os
import traceback
from json import JSONDecodeError

import sys
from typing import List, Tuple, Awaitable, Any, Iterable, Dict

import requests
import websockets
from kubernetes import client, config
from websockets.exceptions import InvalidStatusCode


class TestSession:
    store: str
    app_id: int
    control_plane: str
    test_wheel_path: str


session = TestSession()


def init():
    session.control_plane, session.store = get_urls()
    session.app_id = create_app(session.control_plane)
    session.test_wheel_path = os.path.join(os.path.dirname(__file__), "../dist/runtime_tester-0.0.1-py3-none-any.whl")


def end():
    delete_app(session.control_plane, session.app_id)


def create_app(control_plane=None):
    if control_plane is None:
        control_plane = session.control_plane

    app_url = "http://" + control_plane + "/application"

    ret_val = requests.post(app_url, json={})
    if ret_val.status_code == 200:
        msg = ret_val.json()
        return msg["data"]["app-id"]


def delete_app(control_plane=None, app_id=None):
    if control_plane is None:
        control_plane = session.control_plane
    if app_id is None:
        app_id = session.app_id

    app_url = "http://" + control_plane + "/application"

    ret_val = requests.delete(app_url, json={"app-id": app_id})
    if ret_val.status_code != 200:
        raise Exception("Failed to delete app: " + ret_val.text)


def get_urls():
    control_plane = ""
    store = ""
    config.load_kube_config()
    v1 = client.CoreV1Api()
    svc_list = v1.list_namespaced_service("praas")
    for svc in svc_list.items:
        if svc.metadata.name == "praas-controller":
            control_plane = __get_svc_url(svc)
        elif svc.metadata.name == "praas-function-store":
            store = __get_svc_url(svc)

    return control_plane, store


def __get_svc_url(svc):
    port = svc.spec.ports[0].port

    first_ingress = svc.status.load_balancer.ingress[0]
    if first_ingress.hostname is not None:
        url = first_ingress.hostname
    else:
        url = first_ingress.ip
    return url + ":" + str(port)


def get_pod_proxy(pod_name):
    return f"ws://127.0.0.1:8001/api/v1/namespaces/default/pods/{pod_name}/proxy"


def store_funcs(*func_names: str, wheel_path=None):
    if wheel_path is None:
        wheel_path = session.test_wheel_path
    store_url = "http://" + session.store + "/upload-wheel/"
    functions = ";".join(func_names)

    with open(wheel_path, "rb") as wheel_file:
        form_fields = {"functions": functions}
        upload_resp = requests.post(store_url, data=form_fields, files={"file": wheel_file})
        if not upload_resp.ok:
            raise Exception("Failed to upload functions: " + functions + " - " + upload_resp.text)


def create_process(praas_url=None, app_id=None):
    if app_id is None:
        app_id = session.app_id
    if praas_url is None:
        praas_url = session.control_plane

    proc_create_url = "http://" + praas_url + "/process"
    proc_create_data = {"app-id": app_id, "processes": [{}]}

    ret_val = requests.post(proc_create_url, json=proc_create_data)
    if ret_val.status_code == 200:
        msg = ret_val.json()
        data = msg["data"]["processes"][0]
        return data["pod_name"], data["pid"]


class FuncWaitException(Exception):
    reason: str

    def __init__(self, msg: str, reason: str):
        super().__init__(msg)
        self.reason = reason


async def run_func(
        pod_name: str,
        func_id: int,
        func_name: str,
        args: List[str] = None,
        deps: Dict[int, str] = None,
        success=True,
        mem: str = None,
        cpu: int = None,
        timeout=None,
        wait=True
) -> Any:
    data_plane_addr = get_pod_proxy(pod_name) + "/data-plane"

    ws = None
    retry = 5
    while ws is None and retry > 0:
        try:
            ws = await websockets.connect(data_plane_addr)
        except InvalidStatusCode:
            retry -= 1
            if retry <= 0:
                raise
            await asyncio.sleep(2.1)

    if mem is None:
        mem = "100MiB"
    if cpu is None:
        cpu = 1
    inv_req = {
        "id": func_id,
        "function": func_name,
        "mem": mem,
        "cpu": cpu
    }
    if args is not None:
        inv_req["args"] = args
    if deps is not None:
        inv_req["deps"] = deps

    # print(inv_req)
    send_msg = json.dumps(inv_req)
    await ws.send(send_msg)

    if not wait:
        return ""

    msg = {}
    ret_val = None
    try:
        msg = await __wait_for_op(ws, func_id, "run", timeout)
    except FuncWaitException as e:
        ret_val = e.reason
        if success:
            print(ret_val, file=sys.stderr)
            raise e

    await ws.close()

    if "result" in msg:
        ret_val = msg["result"]

    return ret_val


async def run_func_on_new_process(
        func_id: int,
        func_name: str,
        args: List[str] = None,
        deps: Dict[int, str] = None,
        success=True,
        mem: str = None
) -> Tuple[str, Awaitable]:
    pod_name, _ = create_process(session.control_plane, session.app_id)

    return pod_name, await run_func(
        pod_name,
        func_id,
        func_name,
        args,
        deps,
        success,
        mem
    )


async def __wait_for_op(ws, func_id, op, timeout):
    msg = {}
    waiting = True
    while waiting:
        msg = await asyncio.wait_for(ws.recv(), timeout)
        # print(msg)
        try:
            msg = json.loads(msg)
        except JSONDecodeError as e:
            traceback.print_exc(file=sys.stderr)
            print("Original doc:", e.doc, file=sys.stderr)

        if not msg["success"]:
            ret_val = msg["reason"]
            if ret_val == "Failed to run function to completion":
                ret_val = msg["output"]
            raise FuncWaitException(f"Operation {op} for function {func_id} failed", ret_val)

        waiting = not ("operation" in msg and msg["operation"] == op and msg["func-id"] == func_id)

    return msg
