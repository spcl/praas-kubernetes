#!/usr/bin/env sh

IMG="micro-func"
DIR="$(dirname "$0")"

kubectl delete --ignore-not-found -f "$DIR/func.yaml"
DOCKER_BUILDKIT=1
docker build -t "kind.local/$IMG" "$DIR"
kind load docker-image "kind.local/$IMG:latest"
kubectl apply -f "$DIR/func.yaml"
