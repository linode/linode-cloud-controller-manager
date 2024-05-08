#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

export KUBERNETES_VERSION="$1"
export GOPROXY=off # clusterctl workaround to fetch addons
export CAPI_VERSION="v1.6.3"
export HELM_VERSION="v0.2.1"
export KUBECONFIG="$(realpath "$(dirname "$0")/../kind-management.conf")"

ctlptl create cluster kind \
  --name kind-ccm-management \
  --kubernetes-version ${KUBERNETES_VERSION}

prepare_images() {
  local images="$(echo "$1" | grep -e "^[[:space:]]*image:[?[:space:]]" | awk '{print $2}')"

  echo "${images//[\'\"]}" | xargs -I {} sh -c 'docker pull '{}' ; kind -n ccm-management load docker-image '{}
}

(set +x; prepare_images "$(curl -sfL $(cat $(realpath "$(dirname "$0")/clusterctl.yaml") | grep cluster-api-provider-linode | awk '{print $2}'))")
(set +x ; prepare_images "$(clusterctl init list-images \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  | xargs -I {} echo 'image: '{})")

(cd $(realpath "$(dirname "$0")"); clusterctl init \
  --wait-providers \
  --core cluster-api:${CAPI_VERSION} \
  --addon helm:${HELM_VERSION} \
  --infrastructure akamai-linode \
  --config clusterctl.yaml)
