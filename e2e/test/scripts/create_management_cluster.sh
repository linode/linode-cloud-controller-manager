#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export KUBERNETES_VERSION="$1"
export CAPI_VERSION="v1.6.3"
export HELM_VERSION="v0.1.1-alpha.1"
export CAPL_VERSION="0.1.0"
export KUBECONFIG="$(realpath "$(dirname "$0")/../kind-management.conf")"

ctlptl create cluster kind \
  --name kind-management \
  --kubernetes-version ${KUBERNETES_VERSION}

prepare_images() {
  local images="$(echo "$1" | grep -e "^[[:space:]]*image:[?[:space:]]" | awk '{print $2}')"

  echo "${images//[\'\"]}" | xargs -I {} sh -c 'docker pull '{}' ; kind -n management load docker-image '{}
}

(set +x ; prepare_images "$(cat $(realpath "$(dirname "$0")/infrastructure-linode/${CAPL_VERSION}/infrastructure-components.yaml"))")
(set +x ; prepare_images "$(clusterctl init list-images \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  | xargs -I {} echo 'image: '{})")

(cd $(realpath "$(dirname "$0")"); clusterctl init \
  --wait-providers \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  --infrastructure linode:${CAPL_VERSION} \
  --config clusterctl.yaml)
