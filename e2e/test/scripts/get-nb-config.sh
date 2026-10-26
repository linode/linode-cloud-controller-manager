#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

: "${NBID:?NBID must be set}"

# Queries the API directly: linode-cli's configs-list drops fields missing from
# its response schema (e.g. udp_check_port, udp_session_timeout).
nodebalancer_config_for_port() {
  local configs
  : "${LINODE_TOKEN:?LINODE_TOKEN must be set}"
  configs=$(curl --fail --silent --show-error \
    --header "Authorization: Bearer $LINODE_TOKEN" \
    "${LINODE_URL:-https://api.linode.com}/v4/nodebalancers/$1/configs") || return 1
  jq -ce --argjson port "$2" '.data[] | select(.port == $port)' <<<"$configs"
}

nbconfig=$(retry 3 2 nodebalancer_config_for_port "$NBID" 7070)
printf '%s\n' "$nbconfig"
