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
        offset_bytes = self.client.get_object(Bucket=bucket_name, Key="index.pos", Range=f"bytes={index_num_offset}-{index_num_offset+7}")["Body"].read(8)
        offset = int.from_bytes(offset_bytes, "big")
        response = self.client.get_object(Bucket=bucket_name, Key=object_name, Range=f"bytes={offset}-")
        # Return an open StreamingBody object
        return response['Body']
