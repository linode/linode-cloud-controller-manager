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

## Uninstalling

To remove the CCM:
```bash
kubectl delete -f ccm-linode.yaml
```
