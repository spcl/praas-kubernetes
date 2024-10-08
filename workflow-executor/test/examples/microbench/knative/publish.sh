#!/usr/bin/env sh

IMG="micro-func"
DIR="$(dirname "$0")"
REPO="public.ecr.aws/s4f6z1l2/praas/knative-bench-img"

DOCKER_BUILDKIT=1
docker build -t "kind.local/$IMG" "$DIR"

docker tag "kind.local/$IMG" "$REPO:$IMG"
docker push "$REPO:$IMG"
