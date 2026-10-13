#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

svcname=${1:-svc-test}
: "${NAMESPACE:?NAMESPACE must be set}"
: "${LINODE_TOKEN:?LINODE_TOKEN must be set}"

nbid=$(NAMESPACE="$NAMESPACE" LINODE_TOKEN="$LINODE_TOKEN" "$SCRIPT_DIR/get-nb-id.sh" "$svcname")
vpcconfig=$(retry 3 2 linode_api GET "/v4beta/nodebalancers/$nbid/vpcs")
subnet_id=$(jq -r --argjson nbid "$nbid" '.data[]? | select(.nodebalancer_id == $nbid) | .subnet_id // empty' <<<"$vpcconfig")

printf '%s\n' "$subnet_id"
