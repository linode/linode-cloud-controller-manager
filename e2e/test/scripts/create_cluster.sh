#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

export LINODE_API_TOKEN="$1"
export CLUSTER_NAME="$2"
export IMAGE="$3"


cat > cluster.tf <<EOF
variable "nodes" {
  default = 2
}

module "k8s" {
  source       = "git::https://github.com/linode/terraform-linode-k8s.git"
  k8s_version  = "v1.18.13"
  linode_token = "${LINODE_API_TOKEN}"
  ccm_image    = "${IMAGE}"
  region       = "eu-west"
  cluster_name = "${CLUSTER_NAME}"
  nodes        = var.nodes
}
EOF

terraform workspace new ${CLUSTER_NAME}

terraform init

terraform apply -auto-approve

export KUBECONFIG="$(pwd)/${CLUSTER_NAME}.conf"
