#!/usr/bin/env sh

DIR=$(dirname "$0")
KUBE_CFG="$DIR/eksnative.yaml"
"$DIR/praas-pure/hack/deploy-praas-aws.sh" "$KUBE_CFG"
kubectl --kubeconfig="$KUBE_CFG" apply -f "$DIR/praas-function-store/aws-function-store.yaml"
kubectl --kubeconfig="$KUBE_CFG" apply -f "$DIR/workflow-executor/aws-executor.yaml"