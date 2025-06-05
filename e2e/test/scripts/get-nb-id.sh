#!/bin/bash

set -e

re='^[0-9]+$'

svcname="svc-test"
if [[ -n "$1" ]]; then
    svcname="$1"
fi

hostname=$(kubectl get svc $svcname -n $NAMESPACE -o json | jq -r .status.loadBalancer.ingress[0].hostname)
ip=$(echo $hostname | awk -F'.' '{gsub("-", ".", $1); print $1}')
nbid=$(curl -s \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" \
    -H "X-Filter: {\"ipv4\": \"$ip\"}" \
    "$LINODE_URL/v4/nodebalancers" | jq .data[].id)

if ! [[ $nbid =~ $re ]]; then
    echo "Nodebalancer id [$nbid] is incorrect"
    exit 1
fi

echo $nbid
