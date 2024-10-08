import asyncio
from asyncio import futures

import time

import util


async def test_remote_messaging_put_get():
    util.store_funcs("get_msg", "put_msg")

    test_msg = "Remote Test 1"
    pod1, proc1 = util.create_process()
    pod2, proc2 = util.create_process()

    await util.run_func(pod1, 2, "put_msg", ["0", "1", test_msg], deps={1: proc2})
    read_res = await util.run_func(pod2, 1, "get_msg", ["2"])

    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_remote_messaging_get_put():
    util.store_funcs("get_msg", "put_msg")

    test_msg = "Remote Test 2"

    pod1, proc1 = util.create_process()
    pod2, proc2 = util.create_process()

    read_task = asyncio.create_task(util.run_func(pod1, 1, "get_msg", ["2"]))
    await util.run_func(pod2, 2, "put_msg", ["10", "1", test_msg], deps={1: proc1})
    read_res = await read_task

    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")

    # time.sleep(60)


async def test_local_messaging_put_get():
    util.store_funcs("get_msg", "put_msg")

    pod, proc = util.create_process()

    test_msg = "Test 1"
    await util.run_func(pod, 2, "put_msg", ["0", "1", test_msg], deps={1: proc})
    read_res = await util.run_func(pod, 1, "get_msg", ["2"])

    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_local_messaging_get_put():
    util.store_funcs("get_msg", "put_msg")

    pod, proc = util.create_process()

    test_msg = "Test 2"
    loop = asyncio.get_running_loop()
    read_res = loop.create_task(util.run_func(pod, 1, "get_msg", ["2"]))
    await util.run_func(pod, 2, "put_msg", ["10", "1", test_msg], deps={1: proc})
    read_res = await read_res

    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_retained_message():
    util.store_funcs("get_retain_msg", "put_msg")

    test_msg = "Remote Test 1"
    pod1, proc1 = util.create_process()

    await util.run_func(pod1, 2, "put_msg", ["0", "1", test_msg], deps={1: proc1})

    read_res = await util.run_func(pod1, 1, "get_retain_msg", ["2"], timeout=5)
    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")

    read_res = await util.run_func(pod1, 1, "get_retain_msg", ["2"])
    if read_res != test_msg:
        print("Message was not retained:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_message_not_retained():
    util.store_funcs("get_msg", "put_msg")

    test_msg = "Remote Test 1"
    pod1, proc1 = util.create_process()

    await util.run_func(pod1, 2, "put_msg", ["0", "1", test_msg], deps={1: proc1})

    read_res = await util.run_func(pod1, 1, "get_msg", ["2"], timeout=5)
    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")

    passed = False
    try:
        read_res = await util.run_func(pod1, 1, "get_msg", ["2"], timeout=5)
    except futures.TimeoutError:
        passed = True

    if passed:
        print("[PASSED]")
    else:
        print("Has result:", read_res, " - [FAILED]")


async def test_remote_msg_read_put_get():
    util.store_funcs("get_remote_msg", "put_msg")

    test_msg = "Remote Test 3"
    pod1, proc1 = util.create_process()
    pod2, proc2 = util.create_process()

    await util.run_func(pod1, 2, "put_msg", ["0", "2", test_msg], deps={2: proc1})

    read_res = await util.run_func(pod2, 1, "get_remote_msg", ["2", "2"], deps={2: proc1}, timeout=5)
    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_remote_msg_read_get_put():
    util.store_funcs("get_remote_msg", "put_msg")

    test_msg = "Remote Test 4"

    pod1, proc1 = util.create_process()
    pod2, proc2 = util.create_process()

    read_task = asyncio.create_task(util.run_func(pod1, 1, "get_remote_msg", ["2", "2"], deps={2: proc2}))
    await util.run_func(pod2, 2, "put_msg", ["5", "2", test_msg], deps={2: proc2}, timeout=10)
    read_res = await read_task

    if read_res != test_msg:
        print("Wrong message received:", read_res, "[FAILED]")
    else:
        print("[PASSED]")


async def test_remote_msg_simultaneous():
    util.store_funcs("get_msg", "put_msg")

    test_msg = "Remote Test 5"

    pod1, proc1 = util.create_process()
    pod2, proc2 = util.create_process()

    await util.run_func(pod1, 1, "put_msg", ["0", "4", test_msg], deps={4: proc2}, wait=False)
    await util.run_func(pod2, 2, "put_msg", ["0", "3", test_msg], deps={3: proc1}, wait=False)
    read_res1 = await util.run_func(pod1, 3, "get_msg", ["2"])
    read_res2 = await util.run_func(pod2, 4, "get_msg", ["1"])

    if read_res1 != test_msg:
        print("Wrong message received:", read_res1, "[FAILED]")
        return
    if read_res2 != test_msg:
        print("Wrong message received:", read_res2, "[FAILED]")
        return

    print("[PASSED]")
