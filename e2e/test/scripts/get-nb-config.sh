#!/bin/bash

set -e

nbconfig=$(curl -s \
  -H "Authorization: Bearer $LINODE_TOKEN" \
  -H "Content-Type: application/json" \
  "$LINODE_URL/v4/nodebalancers/$NBID/configs" | jq '.data[] | select(.port == 7070)' || true )

echo $nbconfig
