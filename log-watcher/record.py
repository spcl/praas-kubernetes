import sys

from logs import logs
from usages import resource_usage

if __name__ == "__main__":
    if len(sys.argv) < 1:
        print("Missing command")
        exit(1)
    command = sys.argv[1]
    sys.argv.pop(1)
    globals()[command]()
