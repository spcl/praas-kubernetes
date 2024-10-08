import util


async def test_high_load():
    util.store_funcs("echo")

    proc, _ = util.create_process()
    for i in range(100):
        result = await util.run_func(proc, 1, "echo", ["Hello"])
        if result != "1: Hello":
            print("Wrong result:", result, " - [FAILED]")
            return

    print("[PASSED]")
