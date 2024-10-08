import signal
from multiprocessing.connection import Connection

import importlib
import os
import traceback

import sys
import psutil
from typing import List

import praassdk
from .types import IOAdaptor


class FunctionExecutionError(Exception):
    pass


def func_init(uid: int):
    os.setgid(uid)
    os.setuid(uid)


def timeout_handler(signum, frame):
    raise FunctionExecutionError(f"Function timed out")


def func_runner(
        fid: int,
        func_dir: str,
        wheel_name: str,
        func_name: str,
        args: List[str],
        cores: List[int],
        mem: int,
        conn: Connection,
        timeout: int
):
    with IOAdaptor(conn) as buffer:
        # Environment setup
        p = psutil.Process()
        p.cpu_affinity(cores)
        # p.cpu_affinity([1])
        soft, hard = p.rlimit(psutil.RLIMIT_AS)
        p.rlimit(psutil.RLIMIT_AS, (mem, hard))
        sys.path.append(func_dir)
        signal.signal(signal.SIGALRM, timeout_handler)
        signal.alarm(timeout)

        # Setup the praas sdk
        praassdk.func_id = fid
        praassdk.set_out_buffer(buffer)
        praassdk.set_in_buffer(buffer)

        try:
            # Import and run function
            mod = importlib.import_module(wheel_name)
            func = getattr(mod, func_name)
            result = func(args)
            return result
        except:
            t, v, tb = sys.exc_info()
            msg = f"{func_name}.{fid} - "
            msg += t.__name__ + ": " + str(v)
            printing = True
            new_tb = []
            for frame in traceback.extract_tb(tb):
                f_name = os.path.basename(frame.filename)
                if not printing:
                    in_dir = frame.filename.startswith(func_dir)
                    if in_dir:
                        printing = True

                if printing:
                    frame.filename = f_name
                    new_tb.append(frame)

            if len(new_tb) > 0:
                msg += ", stacktrace:\n"
            msg += "".join(traceback.format_list(new_tb))
            raise FunctionExecutionError(msg)
