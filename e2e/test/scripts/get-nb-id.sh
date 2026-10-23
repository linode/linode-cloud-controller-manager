#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=e2e/test/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

svcname=${1:-svc-test}
: "${NAMESPACE:?NAMESPACE must be set}"

ip=$(kubectl get svc "$svcname" -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

if [[ -z $ip ]]; then
  fail "Service $svcname has no load balancer ingress IP"
  exit 1
fi

nbid=$(retry 3 2 linode_nodebalancer_id_by_ip "$ip")

if [[ ! $nbid =~ ^[0-9]+$ ]]; then
  fail "NodeBalancer id [$nbid] for service $svcname is incorrect"
  exit 1
fi

printf '%s\n' "$nbid"
