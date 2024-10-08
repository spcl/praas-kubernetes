#!/usr/bin/env sh

WORKER="worker"
LEADER="leader"
DIR="$(dirname "$0")"

kubectl delete --ignore-not-found -f "$DIR/nbody.yaml"
DOCKER_BUILDKIT=1
docker build -t "kind.local/nbody-$WORKER" --target "$WORKER" "$DIR"
docker build -t "kind.local/nbody-$LEADER" --target "$LEADER" "$DIR"
kind load docker-image "kind.local/nbody-$WORKER:latest"
kind load docker-image "kind.local/nbody-$LEADER:latest"
kubectl apply -f "$DIR/nbody.yaml"
