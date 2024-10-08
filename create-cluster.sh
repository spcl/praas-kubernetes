#!/bin/bash

kind create cluster --name $KIND_CLUSTER_NAME --config "$(dirname $0)/test-cluster.yaml"

# Loadbalancer Install
echo "Setup load balancer"
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.5/config/manifests/metallb-native.yaml
kubectl wait --for=condition=Ready --all pods -n metallb-system --timeout=120s
kubectl apply -f https://kind.sigs.k8s.io/examples/loadbalancer/metallb-config.yaml
