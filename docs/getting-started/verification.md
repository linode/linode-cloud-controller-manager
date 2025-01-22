# Verification

After installing the CCM, follow these steps to verify it's working correctly.

## Check CCM Pod Status

1. Verify the CCM pods are running:
```bash
kubectl get pods -n kube-system -l app=ccm-linode
```

Expected output:
```
NAME                READY   STATUS    RESTARTS   AGE
ccm-linode-xxxxx    1/1     Running   0          2m
```

2. Check CCM logs:
```bash
kubectl logs -n kube-system -l app=ccm-linode
```

Look for successful initialization messages and no errors.

## Verify Node Configuration

1. Check node annotations:
```bash
kubectl get nodes -o yaml
```

Look for:
- Proper region labels
- Node addresses
- Provider ID

## Test LoadBalancer Service

1. Create a test service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: test-lb
spec:
  type: LoadBalancer
  ports:
    - port: 80
  selector:
    app: test
```

2. Verify NodeBalancer creation:
```bash
kubectl get svc test-lb
```

The service should receive an external IP address.

## Common Issues
- Pods in CrashLoopBackOff: Check logs for API token or permissions issues
- Service stuck in 'Pending': Verify API token has NodeBalancer permissions
- Missing node annotations: Check CCM logs for node controller issues
