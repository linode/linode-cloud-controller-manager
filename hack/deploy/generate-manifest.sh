#!/bin/bash

set -o pipefail -o noclobber -o nounset

die() { echo "$*" 1>&2; exit 1; }

[ "$#" -eq 2 ] || die "First argument must be a Linode APIv4 Personal Access Token with all permissions.
(https://cloud.linode.com/profile/tokens)

Second argument must be a Linode region.
(https://api.linode.com/v4/regions)

Example:
$ ./generate-manifest.sh \$LINODE_API_TOKEN us-east"

ENCODED_TOKEN=$(echo -n $1 | base64)
ENCODED_REGION=$(echo -n $2 | base64)

cat ccm-linode-template.yaml |
sed -e "s|{{ .Values.apiTokenB64 }}|$ENCODED_TOKEN|" |
sed -e "s|{{ .Values.linodeRegionB64 }}|$ENCODED_REGION|" > ccm-linode.yaml
