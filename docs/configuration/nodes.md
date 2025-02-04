# Node Configuration

## Overview

The Node Controller in CCM manages node-specific configurations and lifecycle operations for Kubernetes nodes running on Linode instances.

## Node Labels

The CCM automatically adds the following labels to nodes:

### Topology Labels
Current:
- `topology.kubernetes.io/region`: Linode region (e.g., "us-east")
- `topology.kubernetes.io/zone`: Linode availability zone

Legacy (deprecated):
- `failure-domain.beta.kubernetes.io/region`: Linode region
- `failure-domain.beta.kubernetes.io/zone`: Linode availability zone

### Provider Labels
- `node.kubernetes.io/instance-type`: Linode instance type (e.g., "g6-standard-4")

## Node Annotations

All node annotations must be prefixed with: `node.k8s.linode.com/`

### Available Annotations

| Annotation | Type | Default | Description |
|------------|------|---------|-------------|
| `private-ip` | IPv4 | none | Overrides default detection of Node InternalIP |

### Use Cases

#### Private Network Configuration
```yaml
apiVersion: v1
kind: Node
metadata:
  name: my-node
  annotations:
    node.k8s.linode.com/private-ip: "192.168.1.100"
```

#### VPC Configuration
When using CCM with [Linode VPC](https://www.linode.com/docs/products/networking/vpc/), internal ip will be set to VPC ip. To use a different ip-address as internal ip, you may need to manually configure the node's InternalIP:
```yaml
apiVersion: v1
kind: Node
metadata:
  name: vpc-node
  annotations:
    node.k8s.linode.com/private-ip: "10.0.0.5"
```

## Node Networking

### Private Network Requirements
- NodeBalancers require nodes to have linode specific [private IP addresses](https://techdocs.akamai.com/cloud-computing/docs/managing-ip-addresses-on-a-compute-instance#types-of-ip-addresses)
- Private IPs must be configured in the Linode Cloud Manager or via the API
- The CCM will use private IPs for inter-node communication

### VPC Configuration
When using VPC:
1. Configure network interfaces in Linode Cloud Manager
2. Add appropriate node annotations for private IPs
3. Ensure proper routing configuration
4. Configure security groups if needed

For VPC routing setup, see [Route Configuration](routes.md).

## Node Controller Behavior

### Node Initialization
- Configures node with Linode-specific information
- Sets node addresses (public/private IPs)
- Applies region/zone labels
- Configures node hostnames

### Node Lifecycle Management
- Monitors node health
- Updates node status
- Handles node termination
- Manages node cleanup

### Node Updates
- Updates node labels when region/zone changes
- Updates node addresses when IP configuration changes
- Maintains node conditions based on Linode instance status

For more information:
- [Linode Instance Types](https://www.linode.com/docs/products/compute/compute-instances/plans/)
- [Private Networking](https://www.linode.com/docs/products/networking/private-networking/)
- [VPC Documentation](https://www.linode.com/docs/products/networking/vpc/)
- [Route Configuration](routes.md)
- [Environment Variables](environment.md)
