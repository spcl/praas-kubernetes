import pickle
import time

import sys

import redis

from typing import Any

__redis_client: redis.RedisCluster = None
__self_id = -1


def connect(fid: int, redis_host: str, redis_port: int) -> None:
    global __redis_client
    global __self_id

    __redis_client = redis.RedisCluster(host=redis_host, port=redis_port)
    __self_id = fid


def put(target: int, name: str, msg: Any) -> None:
    if __redis_client is None or __self_id == -1:
        raise Exception("Redis is not connected")

    key = __gen_key(__self_id, target, name)
    serialized_msg = pickle.dumps(msg)
    __redis_client.set(key, serialized_msg)
    print("Set key:", key, file=sys.stderr, flush=True)


def get(source: int, name: str, target: int = -1, retain: bool = False) -> Any:
    if __redis_client is None or __self_id == -1:
        raise Exception("Redis is not connected")

    if target == -1:
        target = __self_id

    key = __gen_key(source, target, name)
    serialized_msg = None
    while serialized_msg is None:
        if retain:
            serialized_msg = __redis_client.get(key)
        else:
            serialized_msg = __redis_client.getdel(key)
            print("Del key:", key, file=sys.stderr, flush=True)
        if serialized_msg is None:
            time.sleep(0.1)

    return pickle.loads(serialized_msg)


def __gen_key(source: int, target: int, name: str):
    return f"{source}-{target}:{name}"
