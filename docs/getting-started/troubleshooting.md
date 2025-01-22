# Troubleshooting

## Common Issues and Solutions

### CCM Pod Issues

#### Pod Won't Start
```bash
kubectl get pods -n kube-system -l app=ccm-linode
kubectl describe pod -n kube-system -l app=ccm-linode
```

Common causes:
- Invalid API token
- Missing RBAC permissions
- Resource constraints

#### Pod Crashes
Check the logs:
```bash
kubectl logs -n kube-system -l app=ccm-linode
```

Common causes:
- API rate limiting
- Network connectivity issues
- Configuration errors

### LoadBalancer Service Issues

#### Service Stuck in Pending
```bash
kubectl describe service <service-name>
```

Check for:
- API token permissions
- NodeBalancer quota limits
- Network configuration

#### Health Checks Failing
Verify:
- Backend pod health
- Service port configuration
- Health check path configuration

### Node Issues

#### Missing Node Labels
```bash
kubectl get nodes --show-labels
```

Verify:
- CCM node controller logs
- Node annotations
- API permissions

#### Network Problems
Check:
- Private IP configuration
- VPC/VLAN setup
- Firewall rules

## Gathering Information

### Useful Commands
```bash
# Get CCM version
kubectl get pods -n kube-system -l app=ccm-linode -o jsonpath='{.items[0].spec.containers[0].image}'

# Check events
kubectl get events -n kube-system

# Get CCM logs with timestamps
kubectl logs -n kube-system -l app=ccm-linode --timestamps
```

### Debug Mode
Set the following environment variable in the CCM deployment:
```yaml
env:
  - name: LINODE_DEBUG
    value: "1"
```

## Getting Help

If issues persist:
1. Join #linode on [Kubernetes Slack](https://kubernetes.slack.com)
2. Check [GitHub Issues](https://github.com/linode/linode-cloud-controller-manager/issues)
3. Submit a new issue with:
   - CCM version
   - Kubernetes version
   - Relevant logs
   - Steps to reproduce
