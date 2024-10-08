#!/usr/bin/env sh

KUBE_CFG="$(dirname "$0")/eksnative.yaml"

kubectl --kubeconfig "$KUBE_CFG" apply -f "$(dirname "$0")/redis.yaml"
kubectl  --kubeconfig "$KUBE_CFG" rollout status statefulset/redis-cluster -n praas
export REDIS_NODES="$(kubectl  --kubeconfig "$KUBE_CFG" get pods  -l app=redis-cluster -n praas -o json | jq -r '.items | map(.status.podIP) | join(":6379 ")'):6379"
echo "Redis nodes: $REDIS_NODES"
kubectl  --kubeconfig "$KUBE_CFG" exec -it redis-cluster-0 -n praas -- sh -c "redis-cli --cluster create --cluster-replicas 1 --cluster-yes $REDIS_NODES"