import asyncio

import util


async def test_assigned_local_invocation():
    util.store_funcs("msg_to_invoked", "invoked", "run_invoke")

    pod, proc = util.create_process()

    await util.run_func(pod, 1, "msg_to_invoked", ["3", "New part"], deps={3: proc}, mem="100MiB")
    result = await util.run_func(pod, 2, "run_invoke", ["local", "invoked"], deps={2: proc}, mem="100MiB")

    if result == "New part":
        print("[PASSED]")
    else:
        print(result)
        print("Wrong result -- [FAILED]")


async def test_local_invocation():
    util.store_funcs("msg_to_invoked", "invoked", "run_invoke")

    pod, proc = util.create_process()

    await util.run_func(pod, 1, "msg_to_invoked", ["3", "New part"], deps={3: proc}, mem="100MiB")
    result = await util.run_func(pod, 2, "run_invoke", [proc, "invoked"], deps={2: proc}, mem="100MiB")

    if result == "New part":
        print("[PASSED]")
    else:
        print(result)
        print("Wrong result -- [FAILED]")


async def test_remote_invocation_proc():
    util.store_funcs("msg_to_invoked", "invoked", "run_invoke")

    pod, proc = util.create_process()

    await util.run_func(pod, 1, "msg_to_invoked", ["3", "New part"], deps={3: proc})
    await asyncio.sleep(15)
    pod2, proc2 = util.create_process()
    result = await util.run_func(pod2, 2, "run_invoke", [proc, "invoked"], deps={2: proc2})

    if result == "New part":
        print("[PASSED]")
    else:
        print(result)
        print("Wrong result -- [FAILED]")
