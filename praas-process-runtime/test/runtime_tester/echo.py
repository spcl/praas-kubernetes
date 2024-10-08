import praassdk


def echo(args):
    msg = str(praassdk.func_id) + ": " + args[0]
    return msg
