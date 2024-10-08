import numpy as np

import torch
from botocore.response import StreamingBody
from torch.utils.data.dataset import Dataset


def from_s3(stream, length, dim, dataset_type):
    if dataset_type == "dense_libsvm":
        return DenseDatasetWithLines(stream, length, dim)
    else:
        raise Exception("dataset type {} is not supported, should be dense_libsvm or sparse_libsvm"
                        .format(dataset_type))


# input is lines
class DenseDatasetWithLines(Dataset):

    def __init__(self, stream: StreamingBody, length, max_dim):
        self.max_dim = max_dim
        self.ins_list = []
        self.label_list = []
        self.ins_list_np = []
        count = 0
        for line in stream.iter_lines():
            if count > length:
                break
            count += 1

            ins = self.parse_line(line.decode("utf-8"))
            if ins is not None:
                self.ins_list.append(ins[0])
                self.ins_list_np.append(ins[0].numpy())
                self.label_list.append(ins[1])
        self.ins_np = np.array(self.ins_list_np)
        self.label_np = np.array(self.label_list).reshape(len(self.label_list), 1)

    def parse_line(self, line):
        splits = line.split(",")
        if len(splits) >= 2:
            label = int(float(splits[0]))
            values = list()
            for item in splits[1:]:
                values.append(float(item))
            vector = torch.tensor(values, dtype=torch.float32)
            return vector, label
        else:
            raise Exception("Split line error: {}".format(line))

    def __getitem__(self, index):
        ins = self.ins_list[index]
        label = self.label_list[index]
        return ins, label

    def __len__(self):
        return len(self.label_list)
