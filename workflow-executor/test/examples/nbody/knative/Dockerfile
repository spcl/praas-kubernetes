# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
# syntax=docker/dockerfile:1
FROM python:3.8-slim-bullseye AS build-stage

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY ./knative /code/knative
COPY ./nbody.py /code/nbody.py
COPY ./main.py /code/main.py


FROM build-stage AS worker
CMD ["uvicorn", "main:worker", "--host", "0.0.0.0", "--port", "8080"]


FROM build-stage AS leader
CMD ["uvicorn", "main:leader_app", "--host", "0.0.0.0", "--port", "8080"]
