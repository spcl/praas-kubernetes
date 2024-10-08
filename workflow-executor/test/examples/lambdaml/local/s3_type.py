import pickle
import boto3


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

    def load(self, object_name, bucket_name=""):
        """Retrieve an object from an Amazon S3 bucket

            :param bucket_name: string
            :param object_name: string
            :return: botocore.response.StreamingBody object. If error, return None.
            """
        response = self.client.get_object(Bucket=bucket_name, Key=object_name)
        # Return an open StreamingBody object
        return response['Body']
