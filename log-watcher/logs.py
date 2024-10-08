import argparse
import atexit
import json

import util


def log_handler(log_entry):
    if type(log_entry) != dict:
        return

    if args.filter.lower() not in log_entry["message"].lower():
        return

    if args.out is not None:
        try:
            file = open(args.out, "a")
            file.write(json.dumps(log_entry))
            file.write("\n")
            file.close()
        except Exception as e:
            print(e)

    print(log_entry)


def logs():
    parser = argparse.ArgumentParser()
    parser.add_argument("--namespace", type=str, required=True)
    parser.add_argument("--filter", type=str, required=True)
    parser.add_argument("--kube-config", type=str, required=False)
    parser.add_argument("--out", type=str, required=False)
    args = parser.parse_args()
    if args.out is not None:
        with open(args.out, "a") as f:
            f.truncate(0)

    pool = util.watch_apps(["bench-master"], args.namespace, log_handler, kube_config=args.kube_config, since_seconds=1)
    atexit.register(lambda: pool.terminate())
    input()
