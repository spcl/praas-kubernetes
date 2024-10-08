#!/bin/bash

# We create the cluster
# eksctl create cluster --kubeconfig eksnative.yaml -f aws-cluster.yaml

# Install knative serving CRDs and controllers
kubectl --kubeconfig=$(dirname $0)/eksnative.yaml apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-crds.yaml
kubectl --kubeconfig=$(dirname $0)/eksnative.yaml wait --for=condition=Established --all crd

kubectl --kubeconfig=$(dirname $0)/eksnative.yaml apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-core.yaml

# Setup networking
kubectl --kubeconfig=$(dirname $0)/eksnative.yaml apply -f https://github.com/knative/net-kourier/releases/download/knative-v1.5.0/kourier.yaml
kubectl --kubeconfig=$(dirname $0)/eksnative.yaml patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress-class":"kourier.ingress.networking.knative.dev"}}'

# Setup DNS
kubectl --kubeconfig=$(dirname $0)/eksnative.yaml apply -f https://github.com/knative/serving/releases/download/knative-v1.5.0/serving-default-domain.yaml



