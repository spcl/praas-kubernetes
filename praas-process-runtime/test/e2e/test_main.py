import argparse
import asyncio
import builtins
import glob
import importlib
import os
import sys
import traceback
from inspect import getmembers, isfunction

import util


orig_print = builtins.print


def test_printer(*args, **kwargs):
    return orig_print(" ", *args, **kwargs)


def load_test_printer():
    builtins.print = test_printer


def load_builtin_printer():
    builtins.print = orig_print


def get_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("-m", "--module", action="append", required=False)
    parser.add_argument("func_filter", nargs="*")
    return parser.parse_args()


async def main():
    args = get_args()

    filtering_funcs = len(args.func_filter) > 0
    filtering_mods = args.module is not None
    if filtering_mods:
        for i in range(len(args.module)):
            if not args.module[i].endswith(".py"):
                args.module[i] += ".py"
    modules = glob.glob(os.path.join(os.path.dirname(__file__), "*.py"))
    excluded_modules = [os.path.basename(__file__), "__init__.py"]
    test_funcs = {}
    for module in modules:
        mod_file = os.path.basename(module)
        if os.path.isfile(module) and mod_file not in excluded_modules:
            if filtering_mods and mod_file not in args.module:
                continue
            mod_name = os.path.basename(module)[:-3]
            mod = importlib.import_module(mod_name)
            test_funcs[mod_name] = list()
            for name, func in getmembers(mod, isfunction):
                is_valid = name.startswith("test_")
                if filtering_funcs:
                    is_valid &= name in args.func_filter
                if is_valid:
                    test_funcs[mod_name].append(func)

    util.init()
    for module in test_funcs.keys():
        if len(test_funcs[module]) == 0:
            continue

        print("---------", module, "---------")
        for test_func in test_funcs[module]:
            try:
                print("-", test_func.__name__)
                load_test_printer()
                await test_func()
            except:
                print("Test:", test_func.__name__, "[FAILED]")
                traceback.print_exc()
                input("Press Enter to continue...")
            finally:
                load_builtin_printer()
    util.end()

asyncio.run(main())
