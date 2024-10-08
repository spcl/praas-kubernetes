import urllib

import sys

import traceback

import time

import pickle
import boto3
from botocore.exceptions import ClientError
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

    def load(self, object_name, bucket_name="", start=0) -> StreamingBody:
        """Retrieve an object from an Amazon S3 bucket
            :param bucket_name: string
            :param start: int
            :param object_name: string
            :return: botocore.response.StreamingBody object. If error, return None.
            """
        if start != 0:
            index_num_offset = start * 8
            offset_bytes = self.client.get_object(Bucket=bucket_name, Key="index.pos", Range=f"bytes={index_num_offset}-{index_num_offset+7}")["Body"].read(8)
            offset = int.from_bytes(offset_bytes, "big")
        else:
            offset = 0

        response = self.client.get_object(Bucket=bucket_name, Key=object_name, Range=f"bytes={offset}-")
        # Return an open StreamingBody object
        return response['Body']

    def load_or_wait(self, object_name, bucket_name="", sleep_time=0.1, time_out=600):
        start_time = time.time()
        while True:
            if time.time() - start_time > time_out:
                return None
            try:
                response = self.client.get_object(Bucket=bucket_name, Key=object_name)
                # Return an open StreamingBody object
                return response['Body']
            except ClientError as e:
                # AllAccessDisabled error == bucket or object not found
                time.sleep(sleep_time)

    def delete(self, key, bucket_name=""):
        if isinstance(key, str):
            try:
                self.client.delete_object(Bucket=bucket_name, Key=key)
            except ClientError:
                traceback.print_exc(file=sys.stderr)
                sys.stderr.flush()
                return False
            return True
        elif isinstance(key, list):
            # Convert list of object names to appropriate data format
            obj_list = [{'Key': obj} for obj in key]

            # Delete the objects
            try:
                self.client.delete_objects(Bucket=bucket_name, Delete={'Objects': obj_list})
            except ClientError:
                traceback.print_exc(file=sys.stderr)
                sys.stderr.flush()
                return False
            return True

    def clear(self, bucket_name=""):
        objects = self.list(bucket_name)
        if objects is not None:
            file_names = []
            for obj in objects:
                file_key = urllib.parse.unquote_plus(obj["Key"], encoding='utf-8')
                file_names.append(file_key)
            if len(file_names) >= 1:
                self.delete(file_names, bucket_name)
        return True

    def list(self, bucket_name=""):
        # Retrieve the list of bucket objects
        try:
            response = self.client.list_objects_v2(Bucket=bucket_name)
        except ClientError:
            traceback.print_exc(file=sys.stderr)
            sys.stderr.flush()
            return None

        # Only return the contents if we found some keys
        if response['KeyCount'] > 0:
            return response['Contents']

        return None
