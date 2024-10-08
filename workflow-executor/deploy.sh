#!/usr/bin/env sh

IMG_NAME="workflow"
DIR="$(dirname "$0")"

kubectl delete --ignore-not-found -f "$DIR/executor.yaml"
DOCKER_BUILDKIT=1
docker build -t "$IMG_NAME" "$DIR"
kind load docker-image "$IMG_NAME:latest"
kubectl apply -f "$DIR/executor.yaml"
