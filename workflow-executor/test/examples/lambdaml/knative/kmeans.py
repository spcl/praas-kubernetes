import sys

import json

import numpy as np
import knative as messaging
import time

import libsvm_dataset, cluster_models
from s3_type import S3Storage
from constants import Prefix


def output(*args):
    print(*args, file=sys.stderr, flush=True)


def sparse_centroid_to_numpy(centroid_sparse_tensor, nr_cluster):
    cent_lst = [centroid_sparse_tensor[i].to_dense().numpy() for i in range(nr_cluster)]
    centroid = np.array(cent_lst)
    return centroid


def centroid_bytes2np(centroid_bytes, n_cluster, data_type, with_error=False):
    centroid_np = np.frombuffer(centroid_bytes, dtype=data_type)
    if with_error:
        centroid_size = centroid_np.shape[0] - 1
        return centroid_np[-1], centroid_np[0:-1].reshape(n_cluster, int(centroid_size / n_cluster))
    else:
        centroid_size = centroid_np.shape[0]
        return centroid_np.reshape(n_cluster, int(centroid_size / n_cluster))


def merge_epoch(local_data, epoch, worker_index, n_workers, name):
    local_slice_key = f"{name}-{worker_index}-{epoch}"

    for i in range(n_workers):
        if i != worker_index:
            messaging.msg.put(i, local_slice_key, local_data)

    for i in range(n_workers):
        if i == worker_index:
            continue
        msg_id = f"{name}-{i}-{epoch}"
        remote_slice = messaging.msg.get(i, msg_id)
        local_data += remote_slice
    return local_data


def kmeans(worker_index, args):
    f_start = time.perf_counter()
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
    n_workers = event["n_workers"]
    batch_size = event["batch_size"]

    key_id = event["key_id"]
    secret_key = event["secret_key"]

    output('data bucket = {}'.format(data_bucket))
    output("file = {}".format(file))
    output('number of workers = {}'.format(n_workers))
    output('worker index = {}'.format(worker_index))
    output('num clusters = {}'.format(n_clusters))

    storage = S3Storage(key_id, secret_key)

    results = dict()

    # Reading data from S3
    read_start = time.perf_counter()
    start = worker_index * batch_size
    file_stream = storage.load(file, start, data_bucket)

    dataset = libsvm_dataset.from_s3(file_stream, batch_size, n_features, dataset_type)
    file_stream.close()
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
    read_time = time.perf_counter() - read_start
    output("read data cost {} s".format(read_time))
    results["data_read"] = read_time
    output("dataset type: {}, dtype: {}, Centroids shape: {}, num_features: {}"
           .format(dataset_type, data_type, centroid_shape, n_features))

    init_centroids_start = time.perf_counter()
    init_msg_name = Prefix.KMeans_Init_Cent + "-1"
    if worker_index == 0:
        if dataset_type == "dense_libsvm":
            centroids = dataset[0:n_clusters]
        elif dataset_type == "sparse_libsvm":
            centroids = sparse_centroid_to_numpy(dataset[0:n_clusters], n_clusters)
        else:
            return

        for other in range(1, n_workers):
            messaging.msg.put(other, init_msg_name, centroids)
        op = "gen_centroid"
        output("generate initial centroids takes {} s"
               .format(time.perf_counter() - init_centroids_start))
    else:
        centroids = messaging.msg.get(0, init_msg_name)
        op = "wait_centroid"
        output("Waiting for initial centroids takes {} s".format(time.perf_counter() - init_centroids_start))
    centroid_time = time.perf_counter() - init_centroids_start
    results[op] = centroid_time

    model = cluster_models.get_model(dataset, centroids, dataset_type, n_features, n_clusters)

    train_start = time.perf_counter()
    epoch_compute = 0
    epoch_sync = 0
    epoch_time = 0
    for epoch in range(n_epochs):
        epoch_start = time.perf_counter()

        # rearrange data points
        model.find_nearest_cluster()

        local_cent = model.get_centroids("numpy").reshape(-1)
        local_cent_error = np.concatenate((local_cent.flatten(), np.array([model.error])))
        epoch_cal_time = time.perf_counter() - epoch_start

        # sync local centroids and error
        epoch_sync_start = time.perf_counter()
        cent_error_merge = merge_epoch(local_cent_error, epoch, worker_index, n_workers, "kmeans")
        cent_merge = cent_error_merge[:-1].reshape(centroid_shape) / float(n_workers)
        error_merge = cent_error_merge[-1] / float(n_workers)

        model.centroids = cent_merge
        model.error = error_merge
        epoch_sync_time = time.perf_counter() - epoch_sync_start

        curr_epoch_time = time.perf_counter() - epoch_start
        output("Epoch[{}] Worker[{}], error = {}, cost {} s, cal cost {} s, sync cost {} s"
               .format(epoch, worker_index, model.error,
                       curr_epoch_time, epoch_cal_time, epoch_sync_time))
        epoch_time += curr_epoch_time
        epoch_compute += epoch_cal_time
        epoch_sync += epoch_sync_time

        if model.error < threshold:
            break

    train_time = time.perf_counter() - train_start
    output("Worker[{}] finishes training: Error = {}, cost {} s"
           .format(worker_index, model.error, train_time))
    results["training"] = train_time
    results["full_epoch_time"] = epoch_time
    results["compute_epoch_time"] = epoch_compute
    results["sync_epoch_time"] = epoch_sync
    results["epochs_done"] = epoch + 1
    results["total_time"] = time.perf_counter() - f_start
    return results
