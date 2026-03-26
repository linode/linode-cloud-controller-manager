#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <manifest-path>" >&2
  exit 1
fi

MANIFEST_PATH="$1"
SUBNET_NAME="${SUBNET_NAME:-}"
IMG_REPO="${IMG%:*}"
IMG_TAG="${IMG##*:}"

tmp_values="$(mktemp)"
trap 'rm -f "${tmp_values}"' EXIT
VPC_NAMES_TEMPLATE='{{ .InfraCluster.spec.vpcRef.name }}'

has_kind() {
  local kind="$1"
  yq e "select(.kind == \"${kind}\") | true" "${MANIFEST_PATH}" | grep -q true
}

patch_ccm_values_template() {
  yq e 'select(.kind == "HelmChartProxy" and .spec.chartName == "ccm-linode").spec.valuesTemplate' "${MANIFEST_PATH}" > "${tmp_values}"

  IMG_REPO="${IMG_REPO}" IMG_TAG="${IMG_TAG}" yq -i e '
    .image.repository = strenv(IMG_REPO) |
    .image.tag = strenv(IMG_TAG) |
    .image.pullPolicy = "Always" |
    .env = ((.env // []) | map(select(.name != "LINODE_API_VERSION")) + [{"name":"LINODE_API_VERSION","value":"v4beta"}])
  ' "${tmp_values}"

  if [ -n "${SUBNET_NAME}" ]; then
    if ! yq e 'has("routeController")' "${tmp_values}" | grep -q true; then
      echo "SUBNET_NAME requires ccm-linode routeController values" >&2
      exit 1
    fi
    SUBNET_NAME="${SUBNET_NAME}" yq -i e '.routeController.subnetNames = strenv(SUBNET_NAME)' "${tmp_values}"
  fi

  if yq e '.routeController | has("vpcNames")' "${tmp_values}" | grep -q true; then
    VPC_NAMES_TEMPLATE="${VPC_NAMES_TEMPLATE}" yq -i e '
      .routeController.vpcNames = strenv(VPC_NAMES_TEMPLATE) |
      .routeController.vpcNames style="single"
    ' "${tmp_values}"
  fi

  TMP_VALUES_PATH="${tmp_values}" yq -i e '
    select(.kind == "HelmChartProxy" and .spec.chartName == "ccm-linode").spec.valuesTemplate = load_str(strenv(TMP_VALUES_PATH))
  ' "${MANIFEST_PATH}"
}

patch_vpc_resources() {
  yq -i e '
    select(.kind == "LinodeVPC") |= (
      del(.spec.ipv6Range) |
      .spec.subnets = [
        {"ipv4": "10.0.0.0/8", "label": "default"},
        {"ipv4": "172.16.0.0/16", "label": "testing"}
      ]
    )
  ' "${MANIFEST_PATH}"

  if [ -n "${SUBNET_NAME}" ]; then
    # The second cluster reuses the existing VPC CR from the first cluster.
    # Reapplying it here can wipe controller-populated subnet IDs back to empty.
    local tmp_manifest
    tmp_manifest="$(mktemp)"
    yq e 'select(.kind != "LinodeVPC")' "${MANIFEST_PATH}" > "${tmp_manifest}"
    mv "${tmp_manifest}" "${MANIFEST_PATH}"
  fi
}

patch_vpc_resources
patch_ccm_values_template
