from typing import List


def reduce(args: List[str]):
    int_arr = map(lambda s: int(s), args)
    return sum(int_arr)
