import sys
# import msgpack

data_file = sys.argv[1]
index_file = sys.argv[2]

offsets = list()
with open(data_file, "r") as df:
    with open(index_file, "wb") as index_f:
        for i in range(11000000):
            offset = df.tell()
            offset_bytes = offset.to_bytes(8, "big")
            index_f.write(offset_bytes)
            line = df.readline()
