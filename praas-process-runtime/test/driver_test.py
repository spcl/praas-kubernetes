import importlib
import os
import traceback

import resource
from typing import TextIO, IO

import sys
import pickle
import praassdk


def handler():
    raise Exception("PraaS functions do not support stdin operations")


sys.argv.pop(0)
module = sys.argv.pop(0)

# Redirect stdout to stderr (make sure user doesn't mess up our pipes)
out_buffer = sys.stdout
sys.stdout = open(os.devnull, 'w')
sys.stdin = type('', (), {})()
for prop, value in vars(TextIO).items():
    if callable(value):
        setattr(sys.stdin, prop, handler)
for prop, value in vars(IO).items():
    if callable(value):
        setattr(sys.stdin, prop, handler)

# Set the current functions id
praassdk.func_id = int(sys.argv.pop(0))

# Set memory limits
mem_limit = int(sys.argv.pop(0))
prev_mem_limit, hard = resource.getrlimit(resource.RLIMIT_AS)
resource.setrlimit(resource.RLIMIT_AS, (mem_limit, hard))

# Get args and run user function
args = sys.argv
result = B""
try:
    runner = importlib.import_module("runtime_tester." + module)
    func = getattr(runner, "run")
    result = pickle.dumps(func(args))
except:
    t, v, tb = sys.exc_info()
    print(t.__name__ + ":", v, file=sys.stderr)
    printing = False
    new_tb = []
    module_f_name = module + ".py"
    for frame in traceback.extract_tb(tb):
        f_name = os.path.basename(frame.filename)
        if f_name == module_f_name:
            printing = True
        if printing:
            frame.filename = f_name
            new_tb.append(frame)
    traceback.print_list(new_tb, file=sys.stderr)
    exit(1)

result = B'\x01' + result
out_buffer.buffer.write(result)
