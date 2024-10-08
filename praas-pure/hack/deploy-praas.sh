#!/usr/bin/env sh

kubectl apply -f "$(dirname "$0")/../config/100-namespace.yaml"
kubectl apply -f "$(dirname "$0")/../config/200-role.yaml"
kubectl apply -f "$(dirname "$0")/../config/300-configmaps/redis.yaml"
kubectl apply -f "$(dirname "$0")/../config/400-deployments/redis.yaml"
"$(dirname "$0")/deploy-redis.sh"
kubectl apply -f "$(dirname "$0")/../config/300-configmaps/praas.yaml"
(cd $(dirname "$0") && ko apply -f ../config/400-deployments/praas.yaml)
