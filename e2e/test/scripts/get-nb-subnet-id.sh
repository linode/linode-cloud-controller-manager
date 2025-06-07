#!/bin/bash

set -e

SCRIPT_DIR=$(dirname "$0")

svcname="svc-test"
if [[ -n "$1" ]]; then
    svcname="$1"
fi

# Get existing dummy nodebalancer id to get VPC config
nbid=$(KUBECONFIG=$KUBECONFIG NAMESPACE=$NAMESPACE LINODE_TOKEN=$LINODE_TOKEN $SCRIPT_DIR/get-nb-id.sh $svcname)

# Get VPC config if it exists
vpcconfig=$(curl -s \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" \
    "$LINODE_URL/v4beta/nodebalancers/$nbid/vpcs")

SUBNET_ID=$(echo $vpcconfig | jq -r ".data[] | select(.nodebalancer_id == $nbid) | .subnet_id")

echo $SUBNET_ID