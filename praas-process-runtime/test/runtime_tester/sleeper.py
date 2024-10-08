import time


def sleep_for(args):
    secs = float(args[0])
    time.sleep(secs)
    return f"Slept for {secs} s"
