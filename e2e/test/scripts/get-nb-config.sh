#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

: "${NBID:?NBID must be set}"

nbconfig=$(retry 3 2 linode_api GET "/v4/nodebalancers/$NBID/configs" | jq -c '.data[]? | select(.port == 7070)')
printf '%s\n' "$nbconfig"
