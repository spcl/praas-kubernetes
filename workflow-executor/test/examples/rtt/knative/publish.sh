#!/usr/bin/env sh

WORKER="help-rtt"
LEADER="measure-rtt"
DIR="$(dirname "$0")"
REPO="public.ecr.aws/s4f6z1l2/praas/knative-bench-img"

DOCKER_BUILDKIT=1
docker build -t "kind.local/$WORKER" --target "$WORKER" "$DIR"
docker build -t "kind.local/$LEADER" --target "$LEADER" "$DIR"

docker tag "kind.local/$WORKER" "$REPO:$WORKER"
docker push "$REPO:$WORKER"


docker tag "kind.local/$LEADER" "$REPO:$LEADER"
docker push "$REPO:$LEADER"
