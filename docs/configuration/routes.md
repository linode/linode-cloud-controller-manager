# Route Configuration

## Overview

The Route Controller manages network routes for pod communication in VPC environments. It ensures proper connectivity between nodes and pods across the cluster by configuring routes in Linode VPC.

## Prerequisites

- Kubernetes cluster running in Linode VPC
- CCM with route controller enabled
- Proper API permissions

## Configuration

### Enable Route Controller

1. Via Helm chart in `values.yaml`:
```yaml
routeController:
  vpcNames: "vpc-prod,vpc-staging"  # Comma separated names of VPCs managed by CCM
  clusterCIDR: "10.0.0.0/8"         # Pod CIDR range
  configureCloudRoutes: true        # Enable route controller
```

2. Via command line flags in CCM deployment:
```yaml
spec:
  template:
    spec:
      containers:
        - name: ccm-linode
          args:
            - --configure-cloud-routes=true
            - --vpc-names=vpc-prod,vpc-staging
            - --cluster-cidr=10.0.0.0/8
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LINODE_ROUTES_CACHE_TTL_SECONDS` | `60` | Default timeout of route cache in seconds |

## Route Management

### Automatic Operations

The Route Controller:
- Creates routes for pod CIDR ranges assigned to nodes
- Updates routes when nodes are added/removed
- Manages route tables in specified VPCs
- Handles route cleanup during node removal
- Maintains route cache for performance

### Route Types

1. **Pod CIDR Routes**
   - Created for each node's pod CIDR allocation
   - Target is node's private IP address
   - Automatically managed based on node lifecycle

2. **VPC Routes**
   - Managed within specified VPCs
   - Enables cross-node pod communication
   - Automatically updated with topology changes

## Best Practices

### CIDR Planning
- Ensure pod CIDR range doesn't overlap with node's VPC ip-address
- Plan for future cluster growth
- Document CIDR allocations

### VPC Configuration
- Use clear, descriptive VPC names
- Configure proper VPC security settings
- Ensure proper API permissions

## Troubleshooting

### Common Issues

1. **Route Creation Failures**
   - Verify API permissions
   - Check for CIDR conflicts
   - Validate VPC configuration
   - Ensure node private IPs are configured

2. **Pod Communication Issues**
   - Verify route table entries
   - Check VPC network ACLs
   - Validate node networking
   - Confirm pod CIDR assignments

## Related Documentation

- [VPC Configuration](https://www.linode.com/docs/products/networking/vpc/)
- [Node Configuration](nodes.md)
- [Environment Variables](environment.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
