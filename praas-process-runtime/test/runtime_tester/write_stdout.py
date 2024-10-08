import praassdk


def write_stdout(args):
    print({"id": praassdk.func_id, "args": args})
    return {"id": praassdk.func_id, "args": args}
