#!/usr/bin/env sh

kubectl apply -f "$(dirname "$0")/../config/config.yaml"
"$(dirname "$0")/deploy-redis.sh"
ko delete --ignore-not-found -f "$(dirname "$0")/../config/controller.yaml" && ko apply -f "$(dirname "$0")/../config/controller.yaml"