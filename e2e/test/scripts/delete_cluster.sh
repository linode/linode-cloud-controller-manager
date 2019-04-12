#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
set -x

cluster_name="$1"
terraform destroy -force

rm cluster.tf
rm ${cluster_name}".conf"

if [[ -d ".terraform" ]]
then
    rm -rf .terraform
fi

if [[ -d "terraform.tfstate.d" ]]
then
    rm -rf terraform.tfstate.d
fi
