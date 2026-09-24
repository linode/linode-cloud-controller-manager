#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart="$repo_root/deploy/chart"

assert_contains() {
  local output=$1
  local expected=$2

  if ! grep -Fq -- "$expected" <<<"$output"; then
    printf 'expected rendered output to contain: %s\n' "$expected" >&2
    exit 1
  fi
}

assert_not_contains() {
  local output=$1
  local unexpected=$2

  if grep -Fq -- "$unexpected" <<<"$output"; then
    printf 'expected rendered output not to contain: %s\n' "$unexpected" >&2
    exit 1
  fi
}

route_disabled="$(
  helm template route-disabled "$chart" \
    --set secretRef.apiTokenRef=apiToken \
    --set secretRef.name=api \
    --set secretRef.regionRef=us-east \
    --set configureCloudRoutes=false \
    --set nodeBalancerBackendIPv4Subnet=10.63.88.0/21 \
    --set nodeBalancerBackendIPv4ReservedRange=10.63.95.252/30
)"
assert_contains "$route_disabled" "--configure-cloud-routes=false"
assert_contains "$route_disabled" "--nodebalancer-backend-ipv4-reserved-range=10.63.95.252/30"
assert_not_contains "$route_disabled" "--enable-route-controller=true"
assert_not_contains "$route_disabled" "--allocate-node-cidrs=true"

route_controller_disabled="$(
  helm template route-controller-disabled "$chart" \
    --set secretRef.apiTokenRef=apiToken \
    --set secretRef.name=api \
    --set secretRef.regionRef=us-east \
    --set routeController.vpcNames=test-vpc \
    --set routeController.subnetNames=test-subnet \
    --set routeController.clusterCIDR=10.0.0.0/16 \
    --set routeController.configureCloudRoutes=false
)"
assert_contains "$route_controller_disabled" "--enable-route-controller=true"
assert_contains "$route_controller_disabled" "--configure-cloud-routes=false"
