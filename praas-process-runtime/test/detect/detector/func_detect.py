from sdk import praas_function


@praas_function
def detected_func(args):
    return args[0]
