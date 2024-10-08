import json

from kmeans import kmeans

args = {
    "file": "higgs-small.csv",
    "data_bucket": "praas-workflow-benchmarks",
    "dataset_type": "dense_libsvm",
    "n_features": 30,
    "n_clusters": 3,
    "n_epochs": 3,
    "threshold": 0.0001,
    "key_id": "***REMOVED***",
    "secret_key": "***REMOVED***"
}

kmeans([json.dumps(args)])
