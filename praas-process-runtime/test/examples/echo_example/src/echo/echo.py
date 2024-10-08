import praassdk


def echo(thing_to_echo):
    ret_val = f"func: {praassdk.func_id}: {thing_to_echo}"
    return ret_val
