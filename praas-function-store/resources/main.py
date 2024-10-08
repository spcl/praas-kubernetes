import importlib
import os
import traceback

from typing import TextIO, IO

import sys
import pickle
import praassdk


def handler():
    raise Exception("PraaS functions do not support stdin operations")


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
praassdk.func_id = int(sys.argv[1])
func_name = sys.argv[2]

# Get args and run user function
args = sys.argv[3:]
out_msg = bytearray()
func_result = None
try:
    runner = importlib.import_module("$wheel")
    func = getattr(runner, func_name)
    func_result = pickle.dumps(func(args))
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
    if len(new_tb) > 0:
        print()
    traceback.print_list(new_tb, file=sys.stderr)
    exit(1)

out_msg.append(praassdk.types.MsgType.RETVAL)
out_msg.extend(func_result)
out_buffer.buffer.write(out_msg)
