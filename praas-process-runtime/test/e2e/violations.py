import util


async def test_exceptions():
    util.store_funcs("throw")

    msg = "Some error message"
    proc, result = await util.run_func_on_new_process(1, "throw", [msg], success=False)
    passed = result.startswith("Exception: " + msg)
    if passed:
        print("[PASSED]")
    else:
        print(result)
        print("Wrong (or no) failure -- [FAILED]")

