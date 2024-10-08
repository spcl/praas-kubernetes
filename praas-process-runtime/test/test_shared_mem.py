import pickle
from multiprocessing import shared_memory

some_obj = "a" * 100
obj_bytes = pickle.dumps(some_obj)
mem_size = len(obj_bytes)
print(mem_size)
shm = shared_memory.SharedMemory(size=mem_size+1, create=True)
shm.buf[1:] = obj_bytes

orig_obj = pickle.loads(shm.buf[1:])
print(orig_obj)
shm.unlink()
