#!/usr/bin/env sh

export KO_DOCKER_REPO="public.ecr.aws/s4f6z1l2/praas"
ko build "$(realpath "$(dirname "$0")/../cmd/control-plane")" -B