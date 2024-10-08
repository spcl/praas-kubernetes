from multiprocessing.connection import Connection

from threading import Lock

from typing import List, Tuple, Dict

import praassdk
from pydantic import BaseModel, Field, validator

from .ws import WebsocketBase


class ResourceManager:
    __lock: Lock
    __cpus: List[bool]
    __allocd_mem: int

    def __init__(self, num_cpus):
        self.__lock = Lock()
        self.__allocd_mem = 0
        self.__cpus = [False] * num_cpus

    def reserve(self, cpu_count: int, mem: int, available_mem: int) -> (bool, List[int]):
        with self.__lock:
            # Check if we have enough resources
            if self.__allocd_mem + mem > available_mem:
                return False, None

            available_cores = []
            for idx, is_used in enumerate(self.__cpus):
                if not is_used:
                    available_cores.append(idx)
            if len(available_cores) < cpu_count:
                return False, None

            # Now we know we have the necessary resources
            self.__allocd_mem += mem
            allocd_cores = list()
            for i in range(cpu_count):
                core_idx = available_cores[i]
                self.__cpus[core_idx] = True
                allocd_cores.append(core_idx)
        return True, allocd_cores

    def free(self, cores: List[int], mem: int):
        with self.__lock:
            for core in cores:
                self.__cpus[core] = False
            self.__allocd_mem -= mem


class Invocation(BaseModel):
    id: int
    function: str
    args: List[str] = []
    deps: Dict[int, str] = dict()
    mem: str = "1GiB"
    cpu: int = 1
    cores: List[int] = Field(default=[], repr=False)

    @validator("mem")
    def valid_memory_str(cls, v):
        mem_to_bytes(v)
        return v

    def get_mem_req(self):
        # We add 30 MiB to it for PEX
        return mem_to_bytes(self.mem)

    def set_cores(self, allocated_cores):
        self.cores = allocated_cores


class PeerList(BaseModel):
    process_id: str
    peers: List[str]


InvocationTask = Tuple[Invocation, WebsocketBase, Lock]


def mem_to_bytes(str_val):
    if str_val.endswith("GiB"):
        cut = 3
        scale = 1073741824
    elif str_val.endswith("MiB"):
        cut = 3
        scale = 1048576
    elif str_val.endswith("KiB"):
        cut = 3
        scale = 1024
    else:
        cut = 1
        scale = 1

    num_str = str_val[:-cut]
    if str_val[-1] != "B":
        raise ValueError("Unknown memory format: " + str_val)

    try:
        value = float(num_str)
    except:
        raise ValueError("Invalid memory request: " + str_val)

    return int(value * scale) + 1


class IOAdaptor(praassdk.protocol.ProcessChannel):
    conn: Connection

    def __init__(self, conn: Connection):
        self.conn = conn

    def write(self, bytes_like):
        self.conn.send_bytes(bytes_like)

    def read(self, n) -> bytes:
        return self.conn.recv_bytes(n)

    def flush(self):
        pass

    def close(self):
        self.conn.close()

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.conn.send_bytes(B"\x01")
        self.conn.close()

    def __enter__(self):
        return self
