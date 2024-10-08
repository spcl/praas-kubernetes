import json

import praassdk as messaging

loop_count = 1


def micro_func(args):
    global loop_count
    loop_count = int(args.pop(0))
    read_from = json.loads(args.pop(0))
    write_to = json.loads(args.pop(0))
    num_array = json.loads(args.pop(0))

    # Add together the input and received arrays
    for src in read_from:
        arr = messaging.msg.get(src, "arr")
        base_arr = arr
        other_arr = num_array
        if len(num_array) > len(arr):
            base_arr = num_array
            other_arr = arr
        for i, num in enumerate(other_arr):
            base_arr[i] += num
        num_array = base_arr

    num_array = list(map(computation, num_array))

    for target in write_to:
        messaging.msg.put(target, "arr", num_array)

    if len(write_to) == 0:
        return num_array
    else:
        return "Done"


def computation(number):
    for _ in range(loop_count):
        number = (number ** 2) % 1000000
    return number
