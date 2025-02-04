# Manual Installation

## Prerequisites
- kubectl configured to access your cluster
- Linode API token
- Target region identified

## Installation Steps

1. Generate the manifest:
```bash
./deploy/generate-manifest.sh $LINODE_API_TOKEN $REGION
```

2. Review the generated manifest:
The script creates `ccm-linode.yaml` containing:
- ServiceAccount
- ClusterRole and ClusterRoleBinding
- Secret with API token
- DaemonSet for the CCM

3. Apply the manifest:
```bash
kubectl apply -f ccm-linode.yaml
```

## Customization

### Environment Variables
You can modify the DaemonSet to include custom environment variables:
```yaml
env:
  - name: LINODE_INSTANCE_CACHE_TTL
    value: "15"
  - name: LINODE_ROUTES_CACHE_TTL_SECONDS
    value: "60"
```

### Resource Limits
Adjust compute resources as needed:
```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 200m
    memory: 256Mi
```

### Prometheus metrics

Cloud Controller Manager exposes metrics by default on port given by
`--secure-port` flag. The endpoint is protected from unauthenticated access by
default. To allow unauthenticated clients (`system:anonymous`) access
Prometheus metrics, use `--authorization-always-allow-paths="/metrics"`
command-line flag.

Linode API calls can be monitored using `ccm_linode_client_requests_total` metric.

## Uninstalling

To remove the CCM:
```bash
kubectl delete -f ccm-linode.yaml
```
