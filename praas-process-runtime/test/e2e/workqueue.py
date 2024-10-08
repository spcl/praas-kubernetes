import asyncio
from datetime import datetime

import util

sleep_time = 2


async def test_workqueue_does_not_block():
    util.store_funcs("sleep_for")

    # Get a new process we can run on
    proc, _ = util.create_process()

    loop = asyncio.get_running_loop()
    start_time = datetime.now()
    sleep1 = loop.create_task(util.run_func(proc, 1, "sleep_for", [str(sleep_time)]))
    sleep2 = loop.create_task(util.run_func(proc, 2, "sleep_for", [str(sleep_time)]))
    sleep3 = loop.create_task(util.run_func(proc, 3, "sleep_for", [str(sleep_time)]))
    sleep4 = loop.create_task(util.run_func(proc, 4, "sleep_for", [str(sleep_time)]))
    await sleep1
    await sleep2
    await sleep3
    await sleep4
    end_time = datetime.now()

    duration = end_time - start_time
    if duration.seconds < 4 * sleep_time:
        print("[PASSED]")
    else:
        print(f"Too long execution time ({duration.seconds} s) -- [FAILED]")


async def test_workqueue_schedules_after_resource_free():
    util.store_funcs("sleep_for")

    # Get a new process we can run on
    proc, _ = util.create_process()

    loop = asyncio.get_running_loop()
    start_time = datetime.now()
    sleep1 = loop.create_task(util.run_func(proc, 1, "sleep_for", [str(sleep_time)], cpu=2))
    sleep2 = loop.create_task(util.run_func(proc, 2, "sleep_for", [str(sleep_time)], cpu=2))
    sleep3 = loop.create_task(util.run_func(proc, 3, "sleep_for", [str(sleep_time)], cpu=2))
    sleep4 = loop.create_task(util.run_func(proc, 4, "sleep_for", [str(sleep_time)], cpu=2))
    await sleep1
    await sleep2
    await sleep3
    await sleep4
    end_time = datetime.now()

    duration = end_time - start_time
    if 4 * sleep_time > duration.seconds >= 2 * sleep_time:
        print("[PASSED]")
    else:
        print(f"Too long execution time ({duration.seconds} s) -- [FAILED]")
