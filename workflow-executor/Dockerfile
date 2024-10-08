# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
FROM python:3.8-slim-bullseye

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

RUN  useradd --no-log-init -r -U -u 1001 worker
USER 1001

COPY ./resources /code/resources
ENV RESOURCES_ROOT /code/resources

COPY ./executor /code/executor

CMD ["uvicorn", "executor.main:app", "--host", "0.0.0.0", "--port", "8080"]