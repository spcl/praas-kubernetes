import sys

import knative as messaging

import time


def measure_rtt(args):
    partner = int(args.pop(0))
    num_runs = int(args.pop(0))
    msg_len = int(args.pop(0))
    msg = "A"*msg_len

    debug("Message is", len(msg), "characters")

    total = 0
    times = []
    for i in range(num_runs):
        out_msg_name = f"{i}-msg-out"
        ret_msg_name = f"{i}-msg-ret"
        start = time.perf_counter()
        messaging.msg.put(partner, out_msg_name, msg)
        messaging.msg.get(partner, ret_msg_name)
        duration = time.perf_counter() - start
        total += duration * 1000
        times.append(duration*1000)

    return {"total": total, "times": times}


def help_rtt(args):
    partner = int(args.pop(0))
    num_runs = int(args.pop(0))

    for i in range(num_runs):
        out_msg_name = f"{i}-msg-out"
        ret_msg_name = f"{i}-msg-ret"
        msg = messaging.msg.get(partner, out_msg_name)
        debug("Message is", len(msg), "characters")
        messaging.msg.put(partner, ret_msg_name, msg)
    return ""


def debug(*args):
    print(*args, file=sys.stderr, flush=True)
