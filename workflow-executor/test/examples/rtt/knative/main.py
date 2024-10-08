from typing import List

from fastapi import FastAPI
from pydantic import BaseModel

import knative
from rtt import help_rtt, measure_rtt


measure = FastAPI()
helper = FastAPI()


class FuncInput(BaseModel):
    id: int
    args: List[str]
    redis_host: str
    redis_port: int


@measure.post("/")
def run_measure(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return measure_rtt(value.args)


@helper.post("/")
def run_helper(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return help_rtt(value.args)
