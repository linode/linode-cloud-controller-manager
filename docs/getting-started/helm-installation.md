# Helm Installation

## Prerequisites
- Helm 3.x installed
- kubectl configured to access your cluster
- Linode API token
- Target region identified

## Installation Steps

1. Add the CCM Helm repository:
```bash
helm repo add ccm-linode https://linode.github.io/linode-cloud-controller-manager/
helm repo update ccm-linode
```

2. Create a values file (values.yaml):
```yaml
apiToken: "your-api-token"
region: "us-east"

# Optional: Configure route controller
routeController:
  vpcNames: ""  # Comma separated VPC names
  clusterCIDR: "10.0.0.0/8"
  configureCloudRoutes: true

# Optional: Assign node internal IPs from VPCs without enabling route controller
# Not required if specified in routeController
vpcNames: "" # Comma separated VPC names

# Optional: Configure shared IP load balancing instead of NodeBalancers (requires Cilium CNI and BGP Control Plane enabled)
sharedIPLoadBalancing:
  loadBalancerType: cilium-bgp
  bgpNodeSelector: cilium-bgp-peering=true
  ipHolderSuffix: ""

# Optional: Allow /metrics scraping without authorization on secure HTTPS port (10253 by default)
allowUnauthorizedMetrics=true
```

3. Install the CCM:
```bash
helm install ccm-linode \
  --namespace kube-system \
  -f values.yaml \
  ccm-linode/ccm-linode
```

## Upgrading

To upgrade an existing installation:
```bash
helm upgrade ccm-linode \
  --namespace kube-system \
  -f values.yaml \
  ccm-linode/ccm-linode
```

## Uninstalling

To remove the CCM:
```bash
helm uninstall ccm-linode -n kube-system
```
