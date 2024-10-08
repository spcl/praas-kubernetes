#!/usr/bin/env sh

DIR="$(dirname "$0")"

kubectl delete --ignore-not-found -f "$DIR/function-store.yaml"
DOCKER_BUILDKIT=1
docker build -t function-store "$DIR"
kind load docker-image function-store:latest
kubectl apply -f "$DIR/function-store.yaml"
