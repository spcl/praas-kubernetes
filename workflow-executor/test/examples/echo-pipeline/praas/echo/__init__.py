import praassdk


def echo(args):
    fid = praassdk.func_id
    msg = ""
    if fid > 1:
        prev = praassdk.msg.get(fid-1, "msg")
        msg += prev
        msg += "+"
    msg += f"{fid} - "
    if len(args) > 0:
        msg += args[0]
        praassdk.msg.put(fid+1, "msg", msg)
    else:
        msg += "end"
        return msg
