import abc
import pickle

# import atomics
# from multiprocessing import shared_memory
from typing import Any, Protocol


class ProcessChannel(Protocol):
    @abc.abstractmethod
    def write(self, bytes_like: bytes):
        pass

    @abc.abstractmethod
    def read(self, n: int) -> bytes:
        pass

    @abc.abstractmethod
    def flush(self):
        pass


def __write_obj(obj: Any, buffer: ProcessChannel, raw=False):
    if not raw:
        obj = pickle.dumps(obj)
    num_bytes = len(obj)

    __write_number(num_bytes, buffer)
    buffer.write(obj)


# def __write_msg(obj: Any, buffer: ProcessChannel):
#     obj = pickle.dumps(obj)
#     num_bytes = len(obj)
#     shared_mem = shared_memory.SharedMemory(size=num_bytes+1, create=True)
#     buf = shared_mem.buf[:1]
#     with atomics.atomicview(buffer=buf, atype=atomics.INT) as a:
#         a.store(1)
#     del buf
#     shared_mem.buf[1:] = obj
#
#     shm_name = shared_mem.name
#     shared_mem.close()
#     __write_obj(shm_name, buffer)
#
#
# def __read_msg(buffer: ProcessChannel):
#     return __read_obj(buffer)


def __write_flag(flag: int, buffer: ProcessChannel):
    as_bytes = bytes([flag])
    buffer.write(as_bytes)


def __write_number(num: int, buffer: ProcessChannel):
    num = num.to_bytes(4, "big", signed=True)
    buffer.write(num)


def __read_number(buffer: ProcessChannel):
    num = buffer.read(4)
    return int.from_bytes(num, "big", signed=True)


def __read_obj(buffer: ProcessChannel, raw=False):
    length = __read_number(buffer)
    obj = buffer.read(length)
    if not raw:
        obj = pickle.loads(obj)
    return obj


def __read_flag(buffer: ProcessChannel):
    flag = buffer.read(1)
    if len(flag) != 1:
        return -1

    return flag[0]
