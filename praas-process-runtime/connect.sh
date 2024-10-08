#!/usr/bin/env sh

echo "PraaS data plane:"
echo "$2" | xclip
websocat "$1"