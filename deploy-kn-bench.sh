#!/usr/bin/env sh

DIR="$(dirname "$0")"

$DIR/deploy-redis.sh
$DIR/praas-function-store/deploy.sh
$DIR/praas-process-runtime/deploy.sh
$DIR/workflow-executor/deploy.sh
$DIR/benchmarks/hack/deploy.sh
