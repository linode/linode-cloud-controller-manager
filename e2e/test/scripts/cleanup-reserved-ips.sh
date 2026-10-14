#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

release_attempt() {
  local reserved_ip=$1
  local response=''
  local response_body
  local http_status
  local curl_status=0
  local api_url=${LINODE_URL:-https://api.linode.com}

  : "${LINODE_TOKEN:?LINODE_TOKEN must be set}"

  if response=$(curl --silent --show-error --write-out $'\n%{http_code}' --request DELETE \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" \
    "${api_url%/}/v4beta/networking/reserved/ips/$reserved_ip"); then
    :
  else
    curl_status=$?
  fi

  http_status=${response##*$'\n'}
  response_body=${response%$'\n'*}
  if [[ ! $http_status =~ ^[0-9]{3}$ ]]; then
    http_status=000
  fi

  case "$http_status" in
    2??|404)
      if ((curl_status != 0)); then
        return "$curl_status"
      fi
      printf 'Reserved IP %s released (HTTP %s)\n' "$reserved_ip" "$http_status"
      return 0
      ;;
  esac

  printf 'Reserved IP %s release failed (HTTP %s): %s\n' \
    "$reserved_ip" "$http_status" "$response_body" >&2
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
  response=$(retry 3 5 linode_api GET "/v4beta/networking/reserved/ips")
  reserved_ips=$(jq -r --arg tag "$E2E_RESERVED_IP_TAG" '.data[]? | select(.tags | index($tag)) | .address' <<<"$response")

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
