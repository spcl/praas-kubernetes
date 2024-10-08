from typing import List

from fastapi import FastAPI
from pydantic import BaseModel

import knative
from func import micro_func


app = FastAPI()


class FuncInput(BaseModel):
    id: int
    args: List[str]
    redis_host: str
    redis_port: int


@app.post("/")
def run_measure(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return micro_func(value.args)
