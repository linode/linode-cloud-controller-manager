#!/bin/bash

set -e

re='^[0-9]+$'

svcname="svc-test"
if [[ -n "$1" ]]; then
    svcname="$1"
fi

ip=$(kubectl get svc "$svcname" -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
nbid=$(curl -s \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" --fail-early --retry 3 \
    -H "X-Filter: {\"ipv4\": \"$ip\"}" \
    "$LINODE_URL/v4/nodebalancers" | jq .data[].id)

if ! [[ $nbid =~ $re ]]; then
    echo "Nodebalancer id [$nbid] is incorrect"
    exit 1
fi

echo $nbid
