import praassdk
import time


def measure_rtt(args):
    partner = int(args.pop(0))
    num_runs = int(args.pop(0))
    msg_len = int(args.pop(0))
    msg = "A"*msg_len

    total = 0
    times = []
    for i in range(num_runs):
        out_msg_name = f"{i}-msg-out"
        ret_msg_name = f"{i}-msg-ret"
        start = time.perf_counter()
        praassdk.msg.put(partner, out_msg_name, msg)
        praassdk.msg.get(partner, ret_msg_name)
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
        msg = praassdk.msg.get(partner, out_msg_name)
        praassdk.msg.put(partner, ret_msg_name, msg)
    return ""
