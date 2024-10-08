# Dockerfile based on the fast api documentation: https://fastapi.tiangolo.com/deployment/docker/
FROM python:3.8-slim-bullseye

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

RUN  useradd --no-log-init --system --user-group -u 1001 worker
RUN chown 1001 /tmp

ENV PEX_ROOT /tmp/.pex

ENV RESOURCES_ROOT /code/resources
COPY ./resources /code/resources

RUN chmod 777 /code/resources/praassdk-0.0.1-py3-none-any.whl

COPY ./builder /code/builder

USER 1001
RUN mkdir -p /tmp/.pex
RUN mkdir -p /tmp/images

CMD ["uvicorn", "builder.build:app", "--host", "0.0.0.0", "--port", "8080"]