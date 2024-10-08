import os

import psutil

from .workqueue import WorkQueue


def get_env_int(env_name: str, fallback: int):
    try:
        value = int(os.getenv(env_name))
    except:
        value = fallback
    return value


work_queue: WorkQueue
cpu_count = get_env_int("CPU_LIMIT", psutil.cpu_count())
self_p = psutil.Process()
mem_bytes = min(psutil.virtual_memory().available, get_env_int("RAM_LIMIT", psutil.virtual_memory().available)) - self_p.memory_info().rss
print("PraaS process has", cpu_count, "cores and", mem_bytes, "bytes of memory available")
# self_p.cpu_affinity([0])
