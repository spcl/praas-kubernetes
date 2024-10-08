#!/usr/bin/env sh

kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/100-namespace.yaml"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/200-role.yaml"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/300-configmaps/redis.yaml"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/../config/400-deployments/redis.yaml"
"$(dirname "$0")/publish.sh"
"$(dirname "$0")/deploy-redis-aws.sh" "$1"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/aws-config.yaml"
kubectl --kubeconfig="$1" apply -f "$(dirname "$0")/aws-praas.yaml"