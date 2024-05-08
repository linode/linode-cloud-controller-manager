#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export CLUSTER_NAME="$1"
export KUBECONFIG="$(realpath "$(dirname "$0")/../kind-management.conf")"

kubectl delete linodecluster -n ${CLUSTER_NAME} --timeout=5m --all --wait
kubectl delete ns ${CLUSTER_NAME} --timeout=5m --force --ignore-not-found
