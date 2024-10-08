import importlib
import sys

wheel = sys.argv[1]
name = sys.argv[2]
args = sys.argv[2:]

mod = importlib.import_module(wheel)
func = getattr(mod, name)
result = func(args)
print(result)
