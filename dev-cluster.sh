#!/bin/bash

# Create cluster
kind create cluster --name $KIND_CLUSTER_NAME --config ../clusterconfig.yaml

# Deploy cert-manager
echo "Deploy cert-manager"
kubectl apply -f ./third_party/cert-manager-latest/cert-manager.yaml
kubectl wait --for=condition=Established --all crd
kubectl wait --for=condition=Available -n cert-manager --all deployments

# Deploy Knative Serving
echo "Install CRDs"
ko apply --selector knative.dev/crd-install=true -Rf config/core/
kubectl wait --for=condition=Established --all crd

echo "Load Knative serving components"
export KIND_CLUSTER_NAME=$KIND_CLUSTER_NAME
ko apply -Rf config/core/
# ko apply -Rf config/hpa-autoscaling
# ko apply -Rf config/ppa-autoscaling 

# Optional steps

# Run post-install job to set up a nice sslip.io domain name.  This only works
# if your Kubernetes LoadBalancer has an IPv4 address.
# ko delete -f config/post-install/default-domain.yaml --ignore-not-found
# ko apply -f config/post-install/default-domain.yaml

# Networking with Kourier
echo "Setup networking"
kubectl apply --filename ../kourier.yaml
kubectl patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress-class":"kourier.ingress.networking.knative.dev"}}'
kubectl patch configmap/config-domain \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"127.0.0.1.sslip.io":""}}'

echo "Setup load balancer"
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.12.1/manifests/metallb.yaml
kubectl wait --for=condition=Ready --all pods -n metallb-system --timeout=120s
kubectl apply -f https://kind.sigs.k8s.io/examples/loadbalancer/metallb-configmap.yaml