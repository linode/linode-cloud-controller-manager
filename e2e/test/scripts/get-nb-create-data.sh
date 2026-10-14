#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

svcname=${1:-svc-test}
: "${LABEL:?LABEL must be set}"
: "${REGION:?REGION must be set}"
: "${NAMESPACE:?NAMESPACE must be set}"
: "${LINODE_TOKEN:?LINODE_TOKEN must be set}"

subnet_id=$(NAMESPACE="$NAMESPACE" LINODE_TOKEN="$LINODE_TOKEN" "$SCRIPT_DIR/get-nb-subnet-id.sh" "$svcname")

if [[ -z $subnet_id ]]; then
  data=$(jq -cn --arg label "$LABEL" --arg region "$REGION" '{label: $label, region: $region}')
else
  data=$(jq -cn \
    --arg label "$LABEL" \
    --arg region "$REGION" \
    --argjson subnet_id "$subnet_id" \
    '{label: $label, region: $region, vpcs: [{subnet_id: $subnet_id}]}')
fi

printf '%s\n' "$data"
