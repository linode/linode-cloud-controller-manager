# Session Affinity

## Overview

Session affinity (also known as sticky sessions) ensures that requests from the same client are consistently routed to the same backend pod. In Kubernetes, sessionAffinity refers to a mechanism that allows a client to always be redirected to the same pod when the client hits a service.

## Configuration

### Basic Setup

Enable session affinity by setting `service.spec.sessionAffinity` to `ClientIP`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: wordpress-lsmnl-wordpress
  namespace: wordpress-lsmnl
  labels:
    app: wordpress-lsmnl-wordpress
spec:
  type: LoadBalancer
  selector:
    app: wordpress-lsmnl-wordpress
  sessionAffinity: ClientIP
```

### Setting Timeout

Configure the maximum session sticky time using `sessionAffinityConfig`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  type: LoadBalancer
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 10800  # 3 hours
```

## Configuration Options

### Session Affinity Types
- `None`: No session affinity (default)
- `ClientIP`: Route based on client's IP address. All requests from the same client IP will be directed to the same pod.

### Timeout Configuration
- `timeoutSeconds`: Duration to maintain affinity
- Default: 10800 seconds (3 hours)
- Valid range: 1 to 86400 seconds (24 hours)
- After the timeout period, client requests may be routed to a different pod

## Related Documentation

- [Service Configuration](annotations.md)
- [LoadBalancer Configuration](loadbalancer.md)
- [Kubernetes Services Documentation](https://kubernetes.io/docs/concepts/services-networking/service/#session-affinity)
- [Service Selectors](https://kubernetes.io/docs/concepts/services-networking/service/#defining-a-service)
