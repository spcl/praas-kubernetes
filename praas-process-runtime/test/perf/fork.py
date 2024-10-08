import datetime
import importlib
import multiprocessing
import os
import pickle
import shutil
import tempfile
import traceback

import psutil
import wget as wget
from zipimport import zipimporter

import sys
from subprocess import Popen

# import psutil as psutil
import time


def run_func(wheel, name, args, cores, mem):
    p = psutil.Process()
    p.cpu_affinity(cores)
    soft, hard = p.rlimit(psutil.RLIMIT_AS)
    p.rlimit(psutil.RLIMIT_AS, (1073741824, hard))

    deps_path = "/home/gyorgy/University/ETH/MasterThesis/praas-process-runtime/test/perf/funcs/exception"
    sys.path.append(deps_path)

    importlib.import_module("summation")
    mod = importlib.import_module(wheel)
    # print(mod)
    func = getattr(mod, name)
    # print(func)
    try:
        result = func(args)
        print("has result", result)
    except:
        t, v, tb = sys.exc_info()
        msg = t.__name__ + ":" + str(v) + ", stacktrace:\n"
        printing = False
        new_tb = []
        for frame in traceback.extract_tb(tb):
            f_name = os.path.basename(frame.filename)
            if not printing:
                in_dir = frame.filename.startswith(deps_path)
                if in_dir:
                    printing = True

            if printing:
                frame.filename = f_name
                new_tb.append(frame)

        msg += "".join(traceback.format_list(new_tb))
        raise Exception(msg)
    # return pickle.dumps(result)


class Manager():
    def __init__(self, conn):
        self.conn = conn

    def __enter__(self):
        self.conn.send("Enter")

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.conn.send("Exit")


def exceptional(conn1, should_raise):
    with Manager(conn1):
        conn1.send("Hello")
    # conn2.close()
    # try:
    #     if should_raise:
    #         raise Exception("bla")
    #     else:
    #         conn1.send("Hello")
    # finally:
    #     conn1.close()


p_start = datetime.datetime.now()
# pool = multiprocessing.Pool(processes=1, maxtasksperchild=15)
pool = multiprocessing.Pool(processes=1)
p_end = datetime.datetime.now()
p_duration = p_end - p_start
print("pool duration:", p_duration.microseconds)


def download_func(name: str):
    start = datetime.datetime.now()
    temp_dir = tempfile.TemporaryDirectory()
    filepath = os.path.join(temp_dir.name, "temp_file.zip")
    func_path = os.path.join("./funcs", name)
    os.makedirs(func_path, exist_ok=True)
    wget.download("http://172.19.255.200/function-image/"+name, filepath, None)
    shutil.unpack_archive(filepath, func_path, "zip")
    temp_dir.cleanup()
    end = datetime.datetime.now()
    duration = end - start
    print("Download time:", duration.microseconds)


def run_measurement_pool():
    start = datetime.datetime.now()
    exceptions = [None]
    def save_exc(e):
        exceptions[0] = e
    run_task = pool.apply_async(run_func, ("summation", "do_thing", ["1", "2", "3"], [0, 1], 10), error_callback=save_exc)
    run_task.wait()
    # result = run_task.get()
    end = datetime.datetime.now()
    duration = end - start
    if not run_task.successful():
        print(format_exception(exceptions[0]))
    # print("Time:", duration.microseconds, "result:", result)
    print("Time:", duration.microseconds)


def format_exception(e: Exception):
    return f"{str(e)}"


def run_measurement_popen():
    start = datetime.datetime.now()
    run_task = Popen(["python", "runner.py", "echo", "echo"])
    run_task.wait()
    end = datetime.datetime.now()
    duration = end - start
    print("Time:", duration.microseconds)


def run_exceptional():
    runtime_pipe, func_pipe = multiprocessing.Pipe()
    args = (func_pipe, True)
    run_task = pool.apply_async(exceptional, args)

    try:
        while True:
            print(runtime_pipe.recv())
        run_task.get()
    except:
        traceback.print_exc()


# download_func("do_thing")
for i in range(1):
    run_exceptional()
    # run_measurement_pool()
    # time.sleep(1)
    # run_measurement_popen()
    # time.sleep(1)

pool.close()
pool.join()

