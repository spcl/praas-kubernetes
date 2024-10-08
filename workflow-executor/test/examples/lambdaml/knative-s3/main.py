from typing import List

from fastapi import FastAPI
from pydantic import BaseModel

from kmeans import kmeans


app = FastAPI()


class FuncInput(BaseModel):
    id: int
    args: List[str]
    redis_host: str
    redis_port: int


@app.post("/")
def run_measure(value: FuncInput):
    return kmeans(value.id, value.args)
