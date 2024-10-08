from typing import List

from fastapi import FastAPI
from pydantic import BaseModel

import knative
from kmeans import kmeans


app = FastAPI()


class FuncInput(BaseModel):
    id: int
    args: List[str]
    redis_host: str
    redis_port: int


@app.post("/")
def run_measure(value: FuncInput):
    knative.msg.connect(value.id, value.redis_host, value.redis_port)
    return kmeans(value.id, value.args)
