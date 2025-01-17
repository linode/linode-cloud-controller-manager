# Service Annotations

## Overview

Service annotations allow you to customize the behavior of your LoadBalancer services. All Service annotations must be prefixed with: `service.beta.kubernetes.io/linode-loadbalancer-`

For implementation details, see:
- [LoadBalancer Configuration](loadbalancer.md)
- [Basic Service Examples](../examples/basic.md)
- [Advanced Configuration Examples](../examples/advanced.md)

## Available Annotations

### Basic Configuration

| Annotation (Suffix) | Values | Default | Description |
|--------------------|--------|---------|-------------|
| `throttle` | `0`-`20` (`0` to disable) | `0` | Client Connection Throttle, which limits the number of subsequent new connections per second from the same client IP |
| `default-protocol` | `tcp`, `http`, `https` | `tcp` | This annotation is used to specify the default protocol for Linode NodeBalancer |
| `default-proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer |
| `port-*` | json object | | Specifies port specific NodeBalancer configuration. See [Port Configuration](#port-specific-configuration) |
| `check-type` | `none`, `connection`, `http`, `http_body` | | The type of health check to perform against back-ends. See [Health Checks](loadbalancer.md#health-checks) |
| `check-path` | string | | The URL path to check on each back-end during health checks |
| `check-body` | string | | Text which must be present in the response body to pass the health check |
| `check-interval` | int | | Duration, in seconds, to wait between health checks |
| `check-timeout` | int (1-30) | | Duration, in seconds, to wait for a health check to succeed |
| `check-attempts` | int (1-30) | | Number of health check failures necessary to remove a back-end |
| `check-passive` | bool | `false` | When `true`, `5xx` status codes will cause the health check to fail |
| `preserve` | bool | `false` | When `true`, deleting a `LoadBalancer` service does not delete the underlying NodeBalancer |
| `nodebalancer-id` | string | | The ID of the NodeBalancer to front the service |
| `hostname-only-ingress` | bool | `false` | When `true`, the LoadBalancerStatus will only contain the Hostname |
| `tags` | string | | A comma separated list of tags to be applied to the NodeBalancer instance |
| `firewall-id` | string | | An existing Cloud Firewall ID to be attached to the NodeBalancer instance. See [Firewall Setup](firewall.md) |
| `firewall-acl` | string | | The Firewall rules to be applied to the NodeBalancer. See [Firewall Configuration](#firewall-configuration) |

### Port Specific Configuration

The `port-*` annotation allows per-port configuration, encoded in JSON. For detailed examples, see [LoadBalancer SSL/TLS Setup](loadbalancer.md#ssltls-configuration).

```yaml
service.beta.kubernetes.io/linode-loadbalancer-port-443: |
  {
    "protocol": "https",
    "tls-secret-name": "my-tls-secret",
    "proxy-protocol": "v2"
  }
```

Available port options:
- `protocol`: Protocol for this port (tcp, http, https)
- `tls-secret-name`: Name of TLS secret for HTTPS. The secret type should be `kubernetes.io/tls`
- `proxy-protocol`: Proxy protocol version for this port

### Deprecated Annotations

| Annotation (Suffix) | Values | Default | Description | Scheduled Removal |
|--------------------|--------|---------|-------------|-------------------|
| `proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer | Q4 2021 |

### Annotation Boolean Values
For annotations with bool value types, the following string values are interpreted as `true`:
- `"1"`
- `"t"`
- `"T"`
- `"true"`
- `"True"`
- `"TRUE"`

Any other values will be interpreted as `false`. For more details, see [strconv.ParseBool](https://golang.org/pkg/strconv/#ParseBool).

## Examples

### Basic HTTP Service
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
    service.beta.kubernetes.io/linode-loadbalancer-check-type: "http"
    service.beta.kubernetes.io/linode-loadbalancer-check-path: "/healthz"
```

### HTTPS Service with TLS
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "protocol": "https",
        "tls-secret-name": "my-tls-secret"
      }
```

### Firewall Configuration
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-acl: |
      {
        "allowList": {
          "ipv4": ["192.168.0.0/16"],
          "ipv6": ["2001:db8::/32"]
        }
      }
```

For more examples and detailed configuration options, see:
- [LoadBalancer Configuration](loadbalancer.md)
- [Firewall Configuration](firewall.md)
- [Basic Service Examples](../examples/basic.md)
- [Advanced Configuration Examples](../examples/advanced.md)
- [Complete Stack Example](../examples/complete-stack.md)

See also:
- [Environment Variables](environment.md)
- [Route Configuration](routes.md)
- [Session Affinity](session-affinity.md)
