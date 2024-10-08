#!/usr/bin/env sh

IMG_NAME="workflow"
REPO="public.ecr.aws/s4f6z1l2/praas/workflow-executor"
DIR="$(dirname "$0")"

DOCKER_BUILDKIT=1
docker build -t "$IMG_NAME" "$DIR"

docker tag "$IMG_NAME" "$REPO:latest"
docker push "$REPO:latest"