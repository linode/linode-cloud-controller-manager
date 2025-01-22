# Advanced Configuration

## Custom Health Checks

Service with custom health check configuration:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web-healthcheck
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-check-type: "http"
    service.beta.kubernetes.io/linode-loadbalancer-check-path: "/healthz"
    service.beta.kubernetes.io/linode-loadbalancer-check-interval: "5"
    service.beta.kubernetes.io/linode-loadbalancer-check-timeout: "3"
    service.beta.kubernetes.io/linode-loadbalancer-check-attempts: "2"
    service.beta.kubernetes.io/linode-loadbalancer-check-passive: "true"
spec:
  type: LoadBalancer
  ports:
    - port: 80
  selector:
    app: web
```

## Firewalled Services

Service with firewall rules:

```yaml
kind: Service
apiVersion: v1
metadata:
  name: restricted-access
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-acl: |
      {
        "allowList": {
          "ipv4": ["192.166.0.0/16", "172.23.41.0/24"],
          "ipv6": ["2001:DB8::/128"]
        }
      }
spec:
  type: LoadBalancer
  selector:
    app: restricted-app
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

## Session Affinity

Service with sticky sessions:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: sticky-service
spec:
  type: LoadBalancer
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 100
  selector:
    app: sticky-app
  ports:
    - port: 80
      targetPort: 8080
```

## Shared IP Load-Balancing

```yaml
apiVersion: v1
kind: Service
metadata:
  name: shared-ip-service
spec:
  type: LoadBalancer
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 8080
---
# Required DaemonSet configuration for shared IP
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ccm-linode
  namespace: kube-system
spec:
  template:
    spec:
      containers:
        - image: linode/linode-cloud-controller-manager:latest
          name: ccm-linode
          env:
            - name: LINODE_URL
              value: https://api.linode.com/v4
          args:
            - --bgp-node-selector=cilium-bgp-peering=true
            - --load-balancer-type=cilium-bgp
            - --ip-holder-suffix=myclustername1
```

## Custom Node Selection

```yaml
apiVersion: v1
kind: Service
metadata:
  name: custom-nodes
spec:
  type: LoadBalancer
  selector:
    app: custom-app
  ports:
    - port: 80
  # Only use nodes with specific labels
  externalTrafficPolicy: Local
---
# Example node with custom annotation
apiVersion: v1
kind: Node
metadata:
  name: custom-node
  annotations:
    node.k8s.linode.com/private-ip: "192.168.1.100"
```

For more examples, see:
- [Service Annotations](../configuration/annotations.md)
- [Firewall Configuration](../configuration/firewall.md)
- [LoadBalancer Configuration](../configuration/loadbalancer.md)
