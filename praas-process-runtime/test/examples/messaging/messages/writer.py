from praassdk import msg


def write_msg(args):
    msg.put(2, args[0])
    return "Done"
