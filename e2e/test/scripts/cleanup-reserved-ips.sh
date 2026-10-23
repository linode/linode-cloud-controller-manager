#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

release_attempt() {
  local reserved_ip=$1
  local response

  if response=$(linode_cli networking reserved-ip-delete "$reserved_ip" --json 2>&1); then
    printf 'Reserved IP %s released\n' "$reserved_ip"
    return 0
  fi

  if [[ $response =~ Request[[:space:]]failed:[[:space:]]404|HTTP[[:space:]]+404 ]]; then
    printf 'Reserved IP %s is already absent\n' "$reserved_ip"
    return 0
  fi

  printf 'Reserved IP %s release failed: %s\n' "$reserved_ip" "$response" >&2
  return 1
}

release_reserved_ip() {
  local reserved_ip=$1

  if [[ ! $reserved_ip =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]]; then
    fail "Invalid reserved IPv4 address: $reserved_ip"
    return 1
  fi

  retry 12 5 release_attempt "$reserved_ip"
}

sweep_reserved_ips() {
  local response
  local reserved_ips
  local reserved_ip

  : "${E2E_RESERVED_IP_TAG:?E2E_RESERVED_IP_TAG must be set}"
  response=$(retry 3 5 linode_cli networking reserved-ips-list --all-rows --json)
  reserved_ips=$(jq -r --arg tag "$E2E_RESERVED_IP_TAG" \
    '.[]? | select((.tags // []) | index($tag)) | .address' <<<"$response")

  while IFS= read -r reserved_ip; do
    if [[ -n $reserved_ip ]]; then
      release_reserved_ip "$reserved_ip"
    fi
  done <<<"$reserved_ips"
}

case "${1:-}" in
  release)
    if [[ $# -ne 2 ]]; then
      printf 'usage: %s release <reserved-ip>\n' "$0" >&2
      exit 2
    fi
    release_reserved_ip "$2"
    ;;
  sweep)
    if [[ $# -ne 1 ]]; then
      printf 'usage: %s sweep\n' "$0" >&2
      exit 2
    fi
    sweep_reserved_ips
    ;;
  *)
    printf 'usage: %s {release <reserved-ip>|sweep}\n' "$0" >&2
    exit 2
    ;;
esac
