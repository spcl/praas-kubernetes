import knative


def echo(fid, args):
    msg = ""
    if fid > 1:
        prev = knative.msg.get(fid-1, "msg")
        msg += prev
        msg += "+"
    msg += f"{fid} - "
    if len(args) > 0:
        msg += args[0]
        knative.msg.put(fid+1, "msg", msg)
    else:
        msg += "end"
        return msg
