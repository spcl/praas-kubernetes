#!/usr/bin/env sh

"$(dirname "$0")/publish.sh"
"$(dirname "$0")/deploy-redis-aws.sh" "$1"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/aws-config.yaml"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/aws-controller.yaml"