#!/bin/bash

set -e

SCRIPT_DIR=$(dirname "$0")

svcname="svc-test"
if [[ -n "$1" ]]; then
    svcname="$1"
fi

# Get existing service's subnet id
SUBNET_ID=$(KUBECONFIG=$KUBECONFIG NAMESPACE=$NAMESPACE LINODE_TOKEN=$LINODE_TOKEN $SCRIPT_DIR/get-nb-subnet-id.sh $svcname)

data="{\"label\": \"$LABEL\", \"region\": \"$REGION\", \"vpcs\": [{\"subnet_id\": $SUBNET_ID}]}"
if [[ -z $SUBNET_ID ]]; then
    data="{\"label\": \"$LABEL\", \"region\": \"$REGION\"}"
fi

echo $data