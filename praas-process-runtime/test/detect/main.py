import importlib
import inspect

import sdk


try:
    mod = importlib.import_module("detector")
    func = getattr(mod, sdk.functions[0])
    print(func(["Hello world"]))
except:
    pass
