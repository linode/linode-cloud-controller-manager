#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

: "${NBID:?NBID must be set}"

nodebalancer_config_for_port() {
  local configs
  configs=$(linode_cli nodebalancers configs-list "$1" --json) || return 1
  jq -ce --argjson port "$2" '.[] | select(.port == $port)' <<<"$configs"
}

nbconfig=$(retry 3 2 nodebalancer_config_for_port "$NBID" 7070)
printf '%s\n' "$nbconfig"
