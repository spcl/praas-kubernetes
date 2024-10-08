import util


async def test_input_output():
    util.store_funcs("echo")

    proc, result = await util.run_func_on_new_process(1, "echo", ["Hello"])
    passed = result == "1: Hello"
    if passed:
        print("[PASSED]")
    else:
        print("Wrong result:", result, " - [FAILED]")
