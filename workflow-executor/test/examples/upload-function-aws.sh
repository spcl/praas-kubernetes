#!/usr/bin/env sh

FNAME=$(basename $1)
STORE_URL=$(kubectl --kubeconfig "$3" get svc -n praas praas-function-store --output jsonpath='{.status.loadBalancer.ingress[0].hostname}')
curl "$STORE_URL/upload-wheel/" -F "file=@$1;filename=$FNAME" -F "functions=$2"
echo ""