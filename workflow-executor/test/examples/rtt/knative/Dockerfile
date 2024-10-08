# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
# syntax=docker/dockerfile:1
FROM python:3.8-slim-bullseye AS build-stage

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY ./knative /code/knative
COPY ./rtt.py /code/rtt.py
COPY ./main.py /code/main.py


FROM build-stage AS help-rtt
CMD ["uvicorn", "main:helper", "--host", "0.0.0.0", "--port", "8080"]


FROM build-stage AS measure-rtt
CMD ["uvicorn", "main:measure", "--host", "0.0.0.0", "--port", "8080"]
