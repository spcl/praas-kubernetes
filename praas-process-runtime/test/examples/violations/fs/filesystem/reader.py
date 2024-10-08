import os


def read(args):
    files = []
    for path, dirs, files in os.walk(args[0]):
        files.extend([os.path.join(path, f) for f in files])
        if len(files) > int(args[1]):
            break
    return files
