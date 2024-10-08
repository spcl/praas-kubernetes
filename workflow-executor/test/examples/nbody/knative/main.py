from typing import List

from fastapi import FastAPI
from pydantic import BaseModel

import knative
from nbody import n_body, leader


class FuncInput(BaseModel):
    id: int
    args: List[str]
    redis_host: str
    redis_port: int


worker = FastAPI()


@worker.post("/")
def run(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return n_body(value.id, value.args)


leader_app = FastAPI()


@leader_app.post("/")
def run(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return leader(value.id, value.args)
