import praassdk
import time


def remote_measure_rtt(args):
    partner = int(args.pop(0))
    num_runs = int(args.pop(0))
    msg_len = int(args.pop(0))
    msg = "A"*msg_len

    total = 0
    times = []
    for i in range(num_runs):
        name = f"iteration-{i}"
        start = time.perf_counter()
        praassdk.msg.put(praassdk.func_id, name, msg)
        praassdk.msg.get(partner, name, target=partner)
        duration = time.perf_counter() - start
        total += duration * 1000
        times.append(duration*1000)

    return {"total": total, "times": times}


def remote_help_rtt(args):
    partner = int(args.pop(0))
    num_runs = int(args.pop(0))

    for i in range(num_runs):
        name = f"iteration-{i}"
        msg = praassdk.msg.get(partner, name, target=partner)
        praassdk.msg.put(praassdk.func_id, name, msg)
    return ""
