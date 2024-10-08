import argparse
import json

from util import watch_resource_usage


def resource_usage():
    parser = argparse.ArgumentParser()
    parser.add_argument("--kube-config", type=str, required=False, default=None)
    parser.add_argument("--out", type=str, required=False)
    args = parser.parse_args()

    watch_resource_usage(handle_entry, kube_config=args.kube_config)


def handle_entry(entry):
    print(json.dumps(entry))
