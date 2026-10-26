#!/usr/bin/env bash

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  return 1
}

retry() {
  if (($# < 3)); then
    fail "usage: retry N DELAY command [args...]"
    return 1
  fi

  local attempts=$1
  local delay=$2
  local command=$3
  local attempt
  local output_file
  shift 2

  if [[ ! $attempts =~ ^[1-9][0-9]*$ ]]; then
    fail "retry requires a positive integer attempt count"
    return 1
  fi

  if [[ ! $delay =~ ^([0-9]+([.][0-9]*)?|[.][0-9]+)$ ]]; then
    fail "retry requires a non-negative numeric delay"
    return 1
  fi

  if ! output_file=$(mktemp); then
    fail "retry could not create a temporary output file"
    return 1
  fi

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if "$@" >"$output_file"; then
      if ! cat "$output_file"; then
        rm -f "$output_file"
        fail "retry could not write the command result to stdout"
        return 1
      fi
      rm -f "$output_file"
      return 0
    fi

    if ((attempt < attempts)) && ! sleep "$delay"; then
      rm -f "$output_file"
      fail "retry could not sleep between attempts"
      return 1
    fi
  done

  rm -f "$output_file"
  fail "retry: $command failed after $attempts attempts (delay ${delay}s)"
}

retry_jq() {
  if (($# < 4)); then
    fail "usage: retry_jq N DELAY JQ_FILTER command [args...]"
    return 1
  fi

  local attempts=$1
  local delay=$2
  local filter=$3
  shift 3

  retry "$attempts" "$delay" json_query "$filter" "$@"
}

json_query() {
  local filter=$1
  local response
  shift

  response=$("$@") || return 1
  jq -ce "$filter" <<<"$response"
}

linode_cli() {
  if (($# == 0)); then
    fail "usage: linode_cli <linode-cli arguments...>"
    return 1
  fi

  : "${LINODE_TOKEN:?LINODE_TOKEN must be set}"

  local api_url=${LINODE_URL:-https://api.linode.com}
  local api_scheme
  local api_host

  case "$api_url" in
    https://*)
      api_scheme=https
      api_host=${api_url#https://}
      ;;
    http://*)
      api_scheme=http
      api_host=${api_url#http://}
      ;;
    *)
      fail "LINODE_URL must use http:// or https://"
      return 1
      ;;
  esac

  api_host=${api_host%%/*}
  if [[ -z $api_host ]]; then
    fail "LINODE_URL must include an API host"
    return 1
  fi

  env \
    "LINODE_CLI_TOKEN=${LINODE_CLI_TOKEN:-$LINODE_TOKEN}" \
    "LINODE_CLI_API_HOST=${LINODE_CLI_API_HOST:-$api_host}" \
    "LINODE_CLI_API_SCHEME=${LINODE_CLI_API_SCHEME:-$api_scheme}" \
    "LINODE_CLI_API_VERSION=${LINODE_CLI_API_VERSION:-v4}" \
    linode-cli "$@"
}

create_reserved_ip() {
  local region=${1:-${REGION:-${LINODE_REGION:-}}}
  local tag=${2:-${E2E_RESERVED_IP_TAG:-}}
  local response
  local address

  if [[ -z $region ]]; then
    fail "LINODE_REGION or REGION must be set"
    return 1
  fi
  if [[ -z $tag ]]; then
    fail "E2E_RESERVED_IP_TAG must be set"
    return 1
  fi

  response=$(retry 3 5 linode_cli networking reserved-ip-add \
    --region "$region" --tags "$tag" --json) || return 1
  address=$(jq -er '.[0].address | strings' <<<"$response") || {
    fail "linode-cli networking reserved-ip-add returned no address"
    return 1
  }

  if ! is_ipv4_address "$address"; then
    fail "linode-cli networking reserved-ip-add returned an invalid IPv4 address: $address"
    return 1
  fi

  printf '%s\n' "$address"
}

is_ipv4_address() {
  local ipv4_re='^((25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})\.){3}(25[0-5]|2[0-4][0-9]|1?[0-9]{1,2})$'
  [[ $1 =~ $ipv4_re ]]
}

create_nodebalancer() {
  local response

  response=$(retry 3 5 linode_cli --raw-body "$1" nodebalancers create --json) || return 1
  jq -er '.[0].id' <<<"$response"
}

create_firewall() {
  local response

  response=$(retry 3 5 linode_cli --raw-body "$1" firewalls create --json) || return 1
  jq -er '.[0].id' <<<"$response"
}

linode_firewall_id_by_label() {
  local firewalls

  firewalls=$(linode_cli firewalls list --all-rows --json) || return 1
  jq -er --arg label "$1" '[.[] | select(.label == $label)][0].id' <<<"$firewalls"
}

linode_nodebalancer_id_by_ip() {
  local nodebalancers

  nodebalancers=$(linode_cli nodebalancers list --ipv4 "$1" --json) || return 1
  jq -er --arg ip "$1" '[.[] | select(.ipv4 == $ip)][0].id' <<<"$nodebalancers"
}

linode_nodebalancer_vpc_config() {
  local configs

  configs=$(linode_cli nodebalancers vpcs-list "$1" --json) || return 1
  jq -c '.[0] // empty' <<<"$configs"
}

# Prints the HTTP status of a GET against the Linode API path $1.
linode_api_status() {
  : "${LINODE_TOKEN:?LINODE_TOKEN must be set}"

  curl -sS -o /dev/null -w '%{http_code}' --max-time 30 \
    -H "Authorization: Bearer $LINODE_TOKEN" \
    "${LINODE_URL:-https://api.linode.com}/${LINODE_CLI_API_VERSION:-v4}/$1"
}

# Succeeds only on a 404; any other status or request error is a failure so
# callers wrapping this in retry keep retrying.
linode_resource_absent() {
  local status

  status=$(linode_api_status "$1") || return 1
  [[ $status == 404 ]]
}

linode_nodebalancer_absent() {
  linode_resource_absent "nodebalancers/$1"
}

linode_firewall_absent() {
  linode_resource_absent "networking/firewalls/$1"
}

linode_firewalls_for_nodebalancer() {
  local firewalls

  firewalls=$(linode_cli firewalls list --all-rows --json) || return 1
  jq -c --argjson id "$1" \
    '[.[] | select(any(.entities[]?; .type == "nodebalancer" and .id == $id))]' <<<"$firewalls"
}

# Records the pod answering at $1 in the caller's `podnames` array and succeeds
# once two distinct pods have been observed. Intended to be wrapped in retry.
observe_distinct_pods() {
  local output
  local existing

  output=$(curl -fsS --max-time 10 "$1" | jq -er '.podName') || return 1
  [[ $output == test-* ]] || return 1

  for existing in "${podnames[@]}"; do
    [[ $existing != "$output" ]] || return 1
  done

  podnames+=("$output")
  ((${#podnames[@]} >= 2))
}
