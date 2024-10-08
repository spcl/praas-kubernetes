#!/bin/bash

# Create cluster
kind create cluster --name $KIND_CLUSTER_NAME --config $HOME/University/ETH/MasterThesis/clusterconfig.yaml

# Loadbalancer Install
echo "Setup load balancer"
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.5/config/manifests/metallb-native.yaml

# Deploy Knative Serving
echo "Install CRDs"
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-crds.yaml
kubectl wait --for=condition=Established --all crd

echo "Install Knative serving components"
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-core.yaml

# Networking setup
echo "Install networking"
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.5.0/kourier.yaml
kubectl patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress-class":"kourier.ingress.networking.knative.dev"}}'


kubectl wait --for=condition=Ready --all pods -n knative-serving --timeout=120s
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-default-domain.yaml

kubectl wait --for=condition=Ready --all pods -n metallb-system --timeout=120s
kubectl apply -f https://kind.sigs.k8s.io/examples/loadbalancer/metallb-config.yaml