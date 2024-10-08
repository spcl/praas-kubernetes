from typing import Any, List

from . import protocol
from .types import MsgType, InvokeRequest

__out_buffer: protocol.ProcessChannel
__in_buffer: protocol.ProcessChannel


def put(target: int, name: str, msg: Any) -> None:
    protocol.__write_flag(MsgType.PUT, __out_buffer)
    protocol.__write_number(target, __out_buffer)
    protocol.__write_obj(name, __out_buffer)
    protocol.__write_obj(msg, __out_buffer)
    __out_buffer.flush()


def broadcast(targets: List[int], name: str, msg: Any) -> None:
    protocol.__write_flag(MsgType.BRDCAST, __out_buffer)
    protocol.__write_obj(targets, __out_buffer)
    protocol.__write_obj(name, __out_buffer)
    protocol.__write_obj(msg, __out_buffer)
    __out_buffer.flush()


def get(source: int, name: str, target: int = -1, retain: bool = False) -> Any:
    if not retain:
        protocol.__write_flag(MsgType.DEL, __out_buffer)
    else:
        protocol.__write_flag(MsgType.GET, __out_buffer)
    protocol.__write_number(source, __out_buffer)
    protocol.__write_number(target, __out_buffer)
    protocol.__write_obj(name, __out_buffer)
    __out_buffer.flush()

    flag = protocol.__read_flag(__in_buffer)
    val = protocol.__read_obj(__in_buffer)
    if flag == MsgType.ERR:
        raise Exception(val)

    return val


def invoke(process: str, func_id: int, func_name: str, args: List[str] = None, cpu=1, mem="1GiB") -> None:
    # Build request object
    if args is None:
        args = []
    invoke_req = InvokeRequest()
    invoke_req.args = args
    invoke_req.id = func_id
    invoke_req.func = func_name
    invoke_req.process = process

    invoke_req.cpu = cpu
    invoke_req.mem = mem

    protocol.__write_flag(MsgType.INVOKE, __out_buffer)
    protocol.__write_obj(invoke_req, __out_buffer)
    __out_buffer.flush()
