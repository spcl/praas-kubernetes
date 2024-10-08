#!/usr/bin/env sh

WORKER="help-rtt"
LEADER="measure-rtt"
DIR="$(dirname "$0")"

kubectl delete --ignore-not-found -f "$DIR/func.yaml"
DOCKER_BUILDKIT=1
docker build -t "kind.local/$WORKER" --target "$WORKER" "$DIR"
docker build -t "kind.local/$LEADER" --target "$LEADER" "$DIR"
kind load docker-image "kind.local/$WORKER:latest"
kind load docker-image "kind.local/$LEADER:latest"
kubectl apply -f "$DIR/func.yaml"
