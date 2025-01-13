#!/bin/bash
set -euo pipefail

# Add bgp peering label to non control plane nodes. Needed to update the shared IP on the nodes
kubectl get nodes --no-headers | grep -v control-plane |\
 awk '{print $1}' | xargs -I {} kubectl label nodes {} cilium-bgp-peering=true --overwrite

# Add RBAC permissions
kubectl patch clusterrole ccm-linode-clusterrole --type='json' -p='[{
  "op": "add",
  "path": "/rules/-",
  "value": {
    "apiGroups": ["cilium.io"],
    "resources": ["ciliumloadbalancerippools", "ciliumbgppeeringpolicies"],
    "verbs": ["get", "list", "watch", "create", "update", "patch", "delete"]
  }
}]'

# Patch DaemonSet
kubectl patch daemonset ccm-linode -n kube-system --type='json' -p='[{
  "op": "add",
  "path": "/spec/template/spec/containers/0/args/-",
  "value": "--bgp-node-selector=cilium-bgp-peering=true"
}, {
  "op": "add",
  "path": "/spec/template/spec/containers/0/args/-",
  "value": "--load-balancer-type=cilium-bgp"
}, {
  "op": "add",
  "path": "/spec/template/spec/containers/0/args/-",
  "value": "--ip-holder-suffix='"${CLUSTER_SUFFIX}"'"
}]'
