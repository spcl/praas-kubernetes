#!/usr/bin/env sh

WORKER="worker"
LEADER="leader"
DIR="$(dirname "$0")"
REPO="public.ecr.aws/s4f6z1l2/praas/knative-bench-img"

DOCKER_BUILDKIT=1
docker build -t "kind.local/$WORKER" --target "$WORKER" "$DIR"
docker build -t "kind.local/$LEADER" --target "$LEADER" "$DIR"

docker tag "kind.local/$WORKER" "$REPO:nbody-$WORKER"
docker push "$REPO:nbody-$WORKER"


docker tag "kind.local/$LEADER" "$REPO:nbody-$LEADER"
docker push "$REPO:nbody-$LEADER"
