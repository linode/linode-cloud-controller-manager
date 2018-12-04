#!/bin/bash

set -o pipefail -o noclobber

die() { echo "$*" 1>&2; exit 1; }

[ "$#" -lt "2" -o "$#" -gt "3" ] && die "First argument must be a Linode APIv4 Personal Access Token with all permissions.
(https://cloud.linode.com/profile/tokens)

Second argument must be a Linode region.
(https://api.linode.com/v4/regions)

Third Argument (Optional) is a Linode NodeBalancer Prefix:
(Up to 9 alpha-numeric characters [A-Za-z0-9_-])

Example:
$ ./generate-manifest.sh \$LINODE_API_TOKEN us-east k8s-"

ENCODED_TOKEN=$(echo -n $1 | base64)
ENCODED_REGION=$(echo -n $2 | base64)
LINODE_NB_PREFIX="$3"


sed \
 -e "s|{{ .Values.apiTokenB64 }}|$ENCODED_TOKEN|" \
 -e "s|{{ .Values.linodeRegionB64 }}|$ENCODED_REGION|" \
 -e "s|{{ .Values.linodeNBPrefix }}|$LINODE_NB_PREFIX|" \
 ccm-linode-template.yaml > ccm-linode.yaml
