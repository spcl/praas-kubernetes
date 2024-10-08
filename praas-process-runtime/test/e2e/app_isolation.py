import asyncio

import util


async def test_app_isolation():
    util.store_funcs("get_msg", "put_msg")

    other_app = util.create_app()

    app1_pod1, app1_proc1 = util.create_process()
    app1_pod2, app1_proc2 = util.create_process()

    app2_pod1, app2_proc1 = util.create_process(app_id=other_app)
    app2_pod2, app2_proc2 = util.create_process(app_id=other_app)

    app1_msg = "Hello from app 1"
    app2_msg = "Hello from app 2"

    await util.run_func(app1_pod1, 1, "put_msg", ["0", "2", app1_msg], deps={2: app1_proc2})
    app2_read_res = asyncio.create_task(util.run_func(app2_pod2, 2, "get_msg", ["1"]))

    await asyncio.sleep(10)
    await util.run_func(app2_pod1, 1, "put_msg", ["0", "2", app2_msg], deps={2: app2_proc2})
    app1_read_res = await util.run_func(app1_pod2, 2, "get_msg", ["1"])
    app2_read_res = await app2_read_res

    if app1_read_res != app1_msg or app2_read_res != app2_msg:
        print("Cross communication between apps -- [FAILED]")
    else:
        print("[PASSED]")

    util.delete_app(app_id=other_app)
