from . import msg
from .protocol import ProcessChannel

func_id: int


def set_out_buffer(buffer: ProcessChannel):
    msg.__out_buffer = buffer


def set_in_buffer(buffer: ProcessChannel):
    msg.__in_buffer = buffer

