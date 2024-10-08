#!/usr/bin/env sh

PRAAS_URL=$1
APP_ID=$2
TEST_TEXT=$3
URL="$PRAAS_URL/process"
echo "$URL"
DATA='{"app-id":'$APP_ID',"processes":[{}]}'
echo "$DATA"
RESULT=$(curl "$URL" -s -X POST --data "$DATA")
echo "$RESULT"
POD_NAME=$(echo "$RESULT" | jq -r ".data.processes[0].pod_name")
WS_URL="ws://127.0.0.1:8001/api/v1/namespaces/default/pods/$POD_NAME/proxy/data-plane"
echo "$WS_URL"
sleep 2
echo "$TEST_TEXT"
./connect.sh "$WS_URL" "$TEST_TEXT"
kubectl logs "$POD_NAME"
