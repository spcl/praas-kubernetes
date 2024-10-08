import time
from concurrent.futures import ThreadPoolExecutor
from threading import Condition

cond = Condition()
wq = list()
output = list()


running = True


def loop_func():
    while running:
        cond.acquire()
        # cond.wait_for(lambda: len(wq) > 0)
        cond.wait()
        output.append(wq.pop(0))
        cond.notify()
        cond.release()


def add_item(item: str):
    # print("Acquire lock")
    wq.append(item)
    cond.acquire()
    # print("Notify")
    cond.notify()
    # print("Release")
    cond.wait()
    cond.release()


pool = ThreadPoolExecutor()
t = pool.submit(loop_func)

add_item("Item 1")
add_item("Item 3")
add_item("Item 2")
running = False
add_item("Item 4")
pool.shutdown(wait=False)
print(output)
