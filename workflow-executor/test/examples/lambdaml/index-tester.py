import sys

import pickle
import boto3
from botocore.response import StreamingBody


class S3Storage:
    def __init__(self, key_id, secret_key):
        session = boto3.Session(
            aws_access_key_id=key_id,
            aws_secret_access_key=secret_key
        )
        self.client = session.client('s3')

    def save(self, src_data, object_name, bucket_name=""):
        """Add an object to an Amazon S3 bucket
        The src_data argument must be of type bytes or a string that references a file specification.

        :param bucket_name: string
        :param object_name: string
        :param src_data: bytes of data or string reference to file spec
        :return: True if src_data was added to dest_bucket/dest_object, otherwise False
        """
        # Construct Body= parameter
        if isinstance(src_data, bytes):
            object_data = src_data
        else:
            object_data = pickle.dumps(src_data)

        # Put the object
        self.client.put_object(Bucket=bucket_name, Key=object_name, Body=object_data)

    def load(self, object_name, start, bucket_name="") -> StreamingBody:
        """Retrieve an object from an Amazon S3 bucket
            :param bucket_name: string
            :param start: int
            :param object_name: string
            :return: botocore.response.StreamingBody object. If error, return None.
            """
        index_num_offset = start * 8
        print(index_num_offset)
        offset_bytes = self.client.get_object(Bucket=bucket_name, Key="index.pos", Range=f"bytes={index_num_offset}-{index_num_offset+7}")["Body"].read(8)
        print("Read dataset from offset bytes:", offset_bytes, file=sys.stderr, flush=True)
        offset = int.from_bytes(offset_bytes, "big")
        print("Read dataset from offset:", offset, file=sys.stderr, flush=True)
        response = self.client.get_object(Bucket=bucket_name, Key=object_name, Range=f"bytes={offset}-")
        # Return an open StreamingBody object
        return response['Body']


store = S3Storage(sys.argv[1], sys.argv[2])
dataset_file = store.load("higgs.csv", 1000, "praas-workflow-benchmarks")
idx = 0
for line in dataset_file.iter_lines():
    if idx > 10:
        break
    idx += 1
    print(line)
