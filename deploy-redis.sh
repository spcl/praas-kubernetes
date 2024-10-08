#!/usr/bin/env sh

kubectl apply -f "$(dirname "$0")/redis.yaml"
kubectl rollout status statefulset/redis-cluster -n praas
export REDIS_NODES="$(kubectl --kubeconfig "$1" get pods  -l app=redis-cluster -n praas -o json | jq -r '.items | map(.status.podIP) | join(":6379 ")'):6379"
echo "Redis nodes: $REDIS_NODES"
kubectl --kubeconfig "$1" exec -it redis-cluster-0 -n praas -- sh -c "redis-cli --cluster create --cluster-replicas 1 --cluster-yes $REDIS_NODES"