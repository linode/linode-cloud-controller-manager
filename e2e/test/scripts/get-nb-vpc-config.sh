#!/bin/bash

set -e

SCRIPT_DIR=$(dirname "$0")

svcname="svc-test"
if [[ -n "$1" ]]; then
    svcname="$1"
fi

# Get the nodebalancer backing the service
nbid=$(KUBECONFIG=$KUBECONFIG NAMESPACE=$NAMESPACE LINODE_TOKEN=$LINODE_TOKEN $SCRIPT_DIR/get-nb-id.sh $svcname)

# Print the VPC config of that nodebalancer, which carries the backend ipv4 range
curl -s \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" --fail-early --retry 3 \
    "$LINODE_URL/v4beta/nodebalancers/$nbid/vpcs" | jq -c ".data[] | select(.nodebalancer_id == $nbid)"
