#!/usr/bin/env sh

DIR="$(dirname "$0")"

$DIR/praas-pure/hack/deploy-praas.sh
$DIR/praas-function-store/deploy.sh
$DIR/praas-process-runtime/deploy.sh
$DIR/workflow-executor/deploy.sh
