# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
# syntax=docker/dockerfile:1
FROM python:3.8-slim-bullseye

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY ./knative /code/knative
COPY ./kmeans.py /code/kmeans.py
COPY ./cluster_models.py /code/cluster_models.py
COPY ./constants.py /code/constants.py
COPY ./libsvm_dataset.py /code/libsvm_dataset.py
COPY ./s3_type.py /code/s3_type.py
COPY ./main.py /code/main.py
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8080"]
