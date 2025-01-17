# Environment Variables

## Overview

Environment variables provide global configuration options for the CCM. These settings affect caching, API behavior, and networking configurations.

## Available Variables

### Cache Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LINODE_INSTANCE_CACHE_TTL` | `15` | Default timeout of instance cache in seconds |
| `LINODE_ROUTES_CACHE_TTL_SECONDS` | `60` | Default timeout of route cache in seconds |

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

## Configuration Methods

### Helm Chart
Configure via `values.yaml`:
```yaml
env:
  - name: LINODE_INSTANCE_CACHE_TTL
    value: "30"
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
```

## Usage Guidelines

### Cache Settings
- Adjust cache TTL based on cluster size and update frequency
- Monitor memory usage when modifying cache settings
- Consider API rate limits when decreasing TTL

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
