# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
# syntax=docker/dockerfile:1
FROM python:3.8-slim-bullseye

RUN mkdir -p /tmp/images
RUN mkdir -p /tmp/workspace
RUN mkdir -p /tmp/pex_root
RUN chmod --recursive 705 /tmp
RUN chmod --recursive 777 /tmp/pex_root
ENV PEX_ROOT /tmp/pex_root

RUN groupadd --system --gid 1001 worker
RUN useradd --no-log-init --system --gid 1001 --uid 1001 worker

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY ./resources /code/resources
ENV RESOURCES_ROOT /code/resources

COPY ./runtime /code/runtime
COPY ./praassdk /code/praassdk

RUN chmod --recursive 700 /code

CMD ["uvicorn", "runtime.main:app", "--host", "0.0.0.0", "--port", "8080"]