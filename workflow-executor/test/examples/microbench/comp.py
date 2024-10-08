def computation(number, loop_count):
    for _ in range(loop_count):
        number = (number ** 2) % 1000000
    return number
