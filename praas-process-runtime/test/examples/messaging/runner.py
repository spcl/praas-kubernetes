import sys
import pickle
from messages import reader

args = sys.argv[1:]
result = pickle.dumps(reader.read_msg(args))
result = B'\x01' + result
sys.stdout.buffer.write(result)
