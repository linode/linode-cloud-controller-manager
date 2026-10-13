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

linode_api() {
  if (($# < 2)); then
    fail "usage: linode_api METHOD /path [curl args...]"
    return 1
  fi

  local method=$1
  local path=$2
  local api_url=${LINODE_URL:-https://api.linode.com}
  local response_file
  local http_status
  local curl_status
  shift 2

  case "$method" in
    GET|POST|PUT|PATCH|DELETE) ;;
    *)
      fail "linode_api does not support HTTP method $method"
      return 1
      ;;
  esac

  if [[ $path != /* ]]; then
    fail "linode_api path must start with /"
    return 1
  fi

  if [[ -z ${LINODE_TOKEN:-} ]]; then
    fail "LINODE_TOKEN must be set"
    return 1
  fi

  if ! response_file=$(mktemp); then
    fail "linode_api could not create a temporary response file"
    return 1
  fi

  if http_status=$(curl --silent --show-error \
    --output "$response_file" \
    --write-out '%{http_code}' \
    --request "$method" \
    --header "Authorization: Bearer $LINODE_TOKEN" \
    --header "Content-Type: application/json" \
    "$@" "${api_url%/}$path"); then
    curl_status=0
  else
    curl_status=$?
  fi

  if ((curl_status != 0)); then
    if [[ -s $response_file ]]; then
      cat "$response_file" >&2
    fi
    rm -f "$response_file"
    return "$curl_status"
  fi

  if [[ $http_status =~ ^2[0-9]{2}$ ]]; then
    if ! cat "$response_file"; then
      rm -f "$response_file"
      fail "linode_api could not write the response to stdout"
      return 1
    fi
    rm -f "$response_file"
    return 0
  fi

  printf 'linode_api: %s %s returned HTTP %s\n' "$method" "$path" "$http_status" >&2
  if [[ -s $response_file ]]; then
    cat "$response_file" >&2
  fi
  rm -f "$response_file"
  return 1
}
