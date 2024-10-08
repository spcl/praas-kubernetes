from typing import List

import praassdk
from praassdk import msg


def run_invoke(args: List[str]):
    new_id = praassdk.func_id + 1
    proc = args.pop(0)
    name = args.pop(0)
    msg.invoke(proc, new_id, name, mem="100MiB")

    return msg.get(new_id, "to_invoker")


def msg_to_invoked(args):
    invoked_id = int(args[0])
    msg.put(invoked_id, "to_invoked", args[1])


def invoked(args):
    invoker_id = praassdk.func_id-1
    msg_part1 = msg.get(1, "to_invoked")
    msg.put(invoker_id, "to_invoker", msg_part1)
