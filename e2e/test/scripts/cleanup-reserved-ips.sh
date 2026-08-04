#!/usr/bin/env bash

set -euo pipefail

release_reserved_ip() {
  local reserved_ip=$1
  local attempt
  local http_status
  local response
  local response_file

  for attempt in {1..12}; do
    response_file=$(mktemp)
    http_status=$(curl -sS -o "$response_file" -w '%{http_code}' --request DELETE \
      -H "Authorization: Bearer $LINODE_TOKEN" \
      -H "Content-Type: application/json" \
      --retry 3 --retry-all-errors \
      "${LINODE_URL}/v4beta/networking/reserved/ips/$reserved_ip" || printf '000')
    response=$(<"$response_file")
    rm -f "$response_file"

    case "$http_status" in
      2??|404)
        echo "Reserved IP $reserved_ip released (HTTP $http_status)"
        return 0
        ;;
    esac

    echo "Reserved IP $reserved_ip release attempt $attempt failed (HTTP $http_status): $response" >&2
    if [[ $attempt -lt 12 ]]; then
      sleep 5
    fi
  done

  return 1
}

sweep_reserved_ips() {
  local response_file

  : "${E2E_RESERVED_IP_TAG:?E2E_RESERVED_IP_TAG must be set}"

  response_file=$(mktemp)
  trap 'rm -f "$response_file"' RETURN

  curl -sS --fail-with-body \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    -H "Content-Type: application/json" \
    "${LINODE_URL}/v4beta/networking/reserved/ips" >"$response_file"

  while IFS= read -r reserved_ip; do
    release_reserved_ip "$reserved_ip"
  done < <(jq -r --arg tag "$E2E_RESERVED_IP_TAG" '.data[] | select(.tags | index($tag)) | .address' "$response_file")
}

case "${1:-}" in
  release)
    if [[ $# -ne 2 ]]; then
      echo "usage: $0 release <reserved-ip>" >&2
      exit 2
    fi
    release_reserved_ip "$2"
    ;;
  sweep)
    if [[ $# -ne 1 ]]; then
      echo "usage: $0 sweep" >&2
      exit 2
    fi
    sweep_reserved_ips
    ;;
  *)
    echo "usage: $0 {release <reserved-ip>|sweep}" >&2
    exit 2
    ;;
esac
