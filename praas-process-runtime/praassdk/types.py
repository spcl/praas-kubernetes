from typing import List


class MsgType:
    RETVAL: int = 1
    PUT: int = 2
    GET: int = 3
    DEL: int = 4
    INVOKE: int = 5
    REP: int = 6
    BRDCAST: int = 7

    ERR: int = 255


class InvokeRequest:
    id: int
    func: str
    process: str
    args: List[str]

    cpu: int
    mem: str
