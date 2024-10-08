import pickle

import sys

import numpy as np
from visualiser import visualise

with open(sys.argv[1], "rb") as f:
    res = f.read()

res = pickle.loads(res)
print(len(res), len(res[0]))
positions = np.zeros((len(res[0]), 3, len(res)))
for i in range(len(res)):
    a = np.matrix(res[i])
    positions[:, :, i] = a
visualise(positions, len(res), 1)
