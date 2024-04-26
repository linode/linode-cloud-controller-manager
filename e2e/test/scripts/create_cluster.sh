#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export LINODE_TOKEN="$1"
export CLUSTER_NAME="$2"
export IMG="$3"
export KUBERNETES_VERSION="$4"
export CAPL_VERSION="0.1.0"
export WORKER_MACHINE_COUNT=2
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2
export KUBECONFIG="$(realpath "$(dirname "$0")/../kind-management.conf")"
export ROOT_DIR="$(git rev-parse --show-toplevel)"

if [[ -z "$5" ]]
then
  export LINODE_REGION="eu-west"
else
  export LINODE_REGION="$5"
fi

(cd ${ROOT_DIR}/deploy ; set +x ; ./generate-manifest.sh ${LINODE_TOKEN} ${LINODE_REGION})

kubectl create ns ${CLUSTER_NAME} ||:
(cd $(realpath "$(dirname "$0")"); clusterctl generate cluster ${CLUSTER_NAME} \
  --target-namespace ${CLUSTER_NAME} \
  --flavor clusterclass-kubeadm \
  --config clusterctl.yaml \
  | kubectl apply --wait -f -)

c=8
until kubectl get secret -n ${CLUSTER_NAME} ${CLUSTER_NAME}-kubeconfig; do
  sleep $(((c--)))
done

clusterctl get kubeconfig -n ${CLUSTER_NAME} ${CLUSTER_NAME} > "$(pwd)/${CLUSTER_NAME}.conf"

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"

c=16
until kubectl version; do
  sleep $(((c--)))
done

c=24
until [[ $(kubectl get no --no-headers | grep Ready | wc -l) == 3 ]]; do
  sleep $(((c--)))
done

c=12
until [[ $(kubectl get ds -n kube-system cilium -o jsonpath="{.status.numberReady}") == 3 ]]; do
  sleep $(((c--)))
done

# For backward compatibility
export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
