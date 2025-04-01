# Environment Variables and Flags

## Overview

The CCM can be configured using environment variables and flags. Environment variables provide global configuration options, while flags control specific features.

## Environment Variables

### Cache Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LINODE_INSTANCE_CACHE_TTL` | `15` | Default timeout of instance cache in seconds |
| `LINODE_ROUTES_CACHE_TTL_SECONDS` | `60` | Default timeout of route cache in seconds |
| `LINODE_METADATA_TTL` | `300` | Default linode metadata timeout in seconds |
| `K8S_NODECACHE_TTL` | `300` | Default timeout of k8s node cache in seconds |

### API Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LINODE_REQUEST_TIMEOUT_SECONDS` | `120` | Default timeout in seconds for http requests to linode API |
| `LINODE_URL` | `https://api.linode.com/v4` | Linode API endpoint |

### Network Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LINODE_EXTERNAL_SUBNET` | "" | Mark private network as external. Example - `172.24.0.0/16` |
| `BGP_CUSTOM_ID_MAP` | "" | Use your own map instead of default region map for BGP |
| `BGP_PEER_PREFIX` | `2600:3c0f` | Use your own BGP peer prefix instead of default one |

## Flags

The CCM supports the following flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--linodego-debug` | `false` | Enables debug output for the LinodeAPI wrapper |
| `--enable-route-controller` | `false` | Enables route_controller for CCM |
| `--enable-token-health-checker` | `false` | Enables Linode API token health checker |
| `--vpc-names` | `""` | Comma separated VPC names whose routes will be managed by route-controller |
| `--subnet-names` | `""` | Comma separated subnet names whose routes will be managed by route-controller (requires vpc-names flag) |
| `--load-balancer-type` | `nodebalancer` | Configures which type of load-balancing to use (options: nodebalancer, cilium-bgp) |
| `--bgp-node-selector` | `""` | Node selector to use to perform shared IP fail-over with BGP |
| `--ip-holder-suffix` | `""` | Suffix to append to the IP holder name when using shared IP fail-over with BGP |
| `--default-nodebalancer-type` | `common` | Default type of NodeBalancer to create (options: common, premium) |
| `--nodebalancer-tags` | `[]` | Linode tags to apply to all NodeBalancers |
| `--nodebalancer-backend-ipv4-subnet` | `""` | ipv4 subnet to use for NodeBalancer backends |
| `--enable-ipv6-for-loadbalancers` | `false` | Set both IPv4 and IPv6 addresses for all LoadBalancer services (when disabled, only IPv4 is used). This can also be configured per-service using the `service.beta.kubernetes.io/linode-loadbalancer-enable-ipv6-ingress` annotation. |
| `--node-cidr-mask-size-ipv4` | `24` | ipv4 cidr mask size for pod cidrs allocated to nodes |
| `--node-cidr-mask-size-ipv6` | `64` | ipv6 cidr mask size for pod cidrs allocated to nodes |

## Configuration Methods

### Helm Chart
Configure via `values.yaml`:
```yaml
env:
  - name: LINODE_INSTANCE_CACHE_TTL
    value: "30"
args:
  - --enable-ipv6-for-loadbalancers
  - --enable-route-controller
```

### Manual Deployment
Add to the CCM DaemonSet:
```yaml
spec:
  template:
    spec:
      containers:
        - name: ccm-linode
          env:
            - name: LINODE_INSTANCE_CACHE_TTL
              value: "30"
          args:
            - --enable-ipv6-for-loadbalancers
            - --enable-route-controller
```

## Usage Guidelines

### Cache Settings
- Adjust cache TTL based on cluster size and update frequency
- Monitor memory usage when modifying cache settings
- Consider API rate limits when decreasing TTL (see [Linode API Rate Limits](@https://techdocs.akamai.com/linode-api/reference/rate-limits))

### API Settings
- Increase timeout for slower network conditions
- Use default API URL unless testing/development required
- Consider regional latency when adjusting timeouts

### Network Settings
- Configure external subnet for custom networking needs
- Use BGP settings only when implementing IP sharing
- Document any custom network configurations

## Troubleshooting

### Common Issues

1. **API Timeouts**
   - Check network connectivity
   - Verify API endpoint accessibility
   - Consider increasing timeout value

2. **Cache Issues**
   - Monitor memory usage
   - Verify cache TTL settings
   - Check for stale data

For more details, see:
- [Installation Guide](../getting-started/installation.md)
- [Troubleshooting Guide](../getting-started/troubleshooting.md)
