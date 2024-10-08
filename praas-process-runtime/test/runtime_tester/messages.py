import time

from praassdk import msg


def get_msg(args):
    src = int(args[0])
    return msg.get(src, "msg")


def get_remote_msg(args):
    src = int(args.pop(0))
    target = int(args.pop(0))
    return msg.get(src, "msg", target=target)


def get_retain_msg(args):
    src = int(args[0])
    return msg.get(src, "msg", retain=True)


def put_msg(args):
    sleep_time = int(args.pop(0))
    target = int(args.pop(0))
    message = args.pop(0)
    time.sleep(sleep_time)
    msg.put(target, "msg", message)
    return "Done"
