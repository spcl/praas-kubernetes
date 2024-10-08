import math

from typing import List, Dict

from pydantic import BaseModel, validator


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

    value = float(num_str)

    return math.ceil(value * scale)


class Message(BaseModel):
    dst: int
    weight: int = 1


class Requirements(BaseModel):
    cpu: int
    mem: str
    instances: List[int]

    @validator("mem")
    def mem_is_recognized(cls, value):
        mem_to_bytes(value)
        return value


class Workflow(BaseModel):
    name: str
    functions: Dict[str, List[int]]
    requirements: List[Requirements]
    communications: Dict[int, List[Message]]
    outputs: List[int]
    inputs: Dict[int, List[str]] = dict()

    def get_functions(self):
        funcs = dict()
        for func_name, func_ids in self.functions.items():
            for func_id in func_ids:
                funcs[func_id] = func_name
        return funcs
