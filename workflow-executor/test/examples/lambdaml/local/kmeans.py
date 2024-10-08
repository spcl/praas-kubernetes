import sys

import json

import numpy as np
import time

import libsvm_dataset
import cluster_models
from s3_type import S3Storage
from constants import Prefix


def output(*args):
    print(*args, file=sys.stderr, flush=True)


def sparse_centroid_to_numpy(centroid_sparse_tensor, nr_cluster):
    cent_lst = [centroid_sparse_tensor[i].to_dense().numpy() for i in range(nr_cluster)]
    centroid = np.array(cent_lst)
    return centroid


def kmeans(args):
    event = json.loads(args[0])
    # dataset
    data_bucket = event['data_bucket']
    file = event['file']
    dataset_type = event["dataset_type"]
    n_features = event['n_features']

    # hyper-parameter
    n_clusters = event['n_clusters']
    n_epochs = event["n_epochs"]
    threshold = event["threshold"]

    key_id = event["key_id"]
    secret_key = event["secret_key"]

    output('data bucket = {}'.format(data_bucket))
    output("file = {}".format(file))
    output('num clusters = {}'.format(n_clusters))

    storage = S3Storage(key_id, secret_key)

    # Reading data from S3
    read_start = time.time()
    with open("../higgs-small.csv") as f:
        lines = f.readlines()
    output("read data cost {} s".format(time.time() - read_start))

    parse_start = time.time()
    dataset = libsvm_dataset.from_lines(lines, n_features, dataset_type)
    if dataset_type == "dense_libsvm":
        dataset = dataset.ins_np
        data_type = dataset.dtype
        centroid_shape = (n_clusters, dataset.shape[1])
    elif dataset_type == "sparse_libsvm":
        dataset = dataset.ins_list
        first_entry = dataset[0].to_dense().numpy()
        data_type = first_entry.dtype
        centroid_shape = (n_clusters, first_entry.shape[1])
    else:
        return
    output("parse data cost {} s".format(time.time() - parse_start))
    output("dataset type: {}, dtype: {}, Centroids shape: {}, num_features: {}"
           .format(dataset_type, data_type, centroid_shape, n_features))

    init_centroids_start = time.time()
    if dataset_type == "dense_libsvm":
        centroids = dataset[0:n_clusters]
    elif dataset_type == "sparse_libsvm":
        centroids = sparse_centroid_to_numpy(dataset[0:n_clusters], n_clusters)
    else:
        return
    output("generate initial centroids takes {} s"
           .format(time.time() - init_centroids_start))

    model = cluster_models.get_model(dataset, centroids, dataset_type, n_features, n_clusters)

    train_start = time.time()
    for epoch in range(n_epochs):
        epoch_start = time.time()
        # rearrange data points
        model.find_nearest_cluster()
        epoch_cal_time = time.time() - epoch_start

        local_cent = model.get_centroids("numpy").reshape(-1)
        local_cent_error = np.concatenate((local_cent.flatten(), np.array([model.error])))

        output("Err merge len:", len(local_cent_error), centroid_shape)

        output("Epoch[{}], error = {}, cost {} s, cal cost {} s"
               .format(epoch, model.error, time.time() - epoch_start, epoch_cal_time))

        if model.error < threshold:
            break

    output("Worker finishes training: Error = {}, cost {} s"
           .format(model.error, time.time() - train_start))
    return
