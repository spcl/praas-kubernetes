#!/usr/bin/env sh

DIR="$(dirname "$0")"

DOCKER_BUILDKIT=1
docker build -t kind.local/container-test "$DIR"
kind load docker-image kind.local/container-test:latest