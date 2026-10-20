#!/usr/bin/env bash
set -euo pipefail

assert_contains() {
  case "$rendered" in
    *"$1"*) ;;
    *) printf 'expected rendered chart to contain: %s\n' "$1" >&2; exit 1 ;;
  esac
}

assert_no_scientific_ids() {
  case "$rendered" in
    *'1.054213e+06'*|*'1.054214e+06'*|*'1.054215e+06'*)
      printf 'rendered chart contains a scientific-notation ID\n' >&2
      exit 1
      ;;
  esac
}

rendered=$(helm template ccm-linode deploy/chart \
  --set-json 'vpcIDs=1.054213e+06' \
  --set-json 'subnetIDs=1.054214e+06' \
  --set-json 'nodeBalancerBackendIpv4SubnetID=1.054215e+06')
assert_contains '"--vpc-ids=1054213"'
assert_contains '"--subnet-ids=1054214"'
assert_contains '"--nodebalancer-backend-ipv4-subnet-id=1054215"'
assert_no_scientific_ids

rendered=$(helm template ccm-linode deploy/chart \
  --set-json 'routeController.vpcIDs=1.054213e+06' \
  --set-json 'routeController.subnetIDs=1.054214e+06' \
  --set 'routeController.clusterCIDR=10.192.0.0/10')
assert_contains '"--vpc-ids=1054213"'
assert_contains '"--subnet-ids=1054214"'
assert_no_scientific_ids

rendered=$(helm template ccm-linode deploy/chart \
  --set-string 'vpcIDs=1054213\,1054214' \
  --set-string 'subnetIDs=1054215\,1054216')
assert_contains '"--vpc-ids=1054213,1054214"'
assert_contains '"--subnet-ids=1054215,1054216"'
