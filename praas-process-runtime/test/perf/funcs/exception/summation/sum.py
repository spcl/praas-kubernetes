from .another_file import gen_new_array


def do_thing(args):
    some_array = [int(elem) for elem in args]
    other_array = gen_new_array(some_array)
    return some_array + other_array