#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

export LINODE_API_TOKEN="$1"
export CLUSTER_NAME="$2"
export IMAGE="$3"
export K8S_VERSION="$4"

if [[ -z "$5" ]]
then
  export REGION="eu-west"
else
  export REGION="$5"
fi

cat > cluster.tf <<EOF
variable "nodes" {
  default = 2
}

module "k8s" {
  source       = "git::https://github.com/linode/terraform-linode-k8s.git"
  k8s_version  = "${K8S_VERSION}"
  linode_token = "${LINODE_API_TOKEN}"
  ccm_image    = "${IMAGE}"
  region       = "${REGION}"
  cluster_name = "${CLUSTER_NAME}"
  nodes        = var.nodes
}
EOF

terraform workspace new ${CLUSTER_NAME}

terraform init

terraform apply -auto-approve

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
