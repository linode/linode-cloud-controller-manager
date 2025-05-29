# Service Annotations

## Overview

Service annotations allow you to customize the behavior of your LoadBalancer services. All Service annotations must be prefixed with: `service.beta.kubernetes.io/linode-loadbalancer-`

For implementation details, see:
- [LoadBalancer Configuration](loadbalancer.md)
- [Basic Service Examples](../examples/basic.md)
- [Advanced Configuration Examples](../examples/advanced.md)

NOTE:
The keys and the values in [annotations must be strings](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/). In other words, you cannot use numeric, boolean, list or other types for either the keys or the values. Hence, one should double quote them when specifying in annotations to convert them to string.

## Available Annotations

### Basic Configuration

| Annotation (Suffix) | Values | Default | Description |
|--------------------|--------|---------|-------------|
| `throttle` | `0`-`20` (`0` to disable) | `0` | Client Connection Throttle, which limits the number of subsequent new connections per second from the same client IP |
| `default-protocol` | `tcp`, `udp`, `http`, `https` | `tcp` | This annotation is used to specify the default protocol for Linode NodeBalancer |
| `default-proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer |
| `default-algorithm` | `roundrobin`, `leastconn`, `source`, `ring_hash` | `roundrobin` | This annotation is used to specify the default algorithm for Linode NodeBalancer |
| `default-stickiness` | `none`, `session`, `table`, `http_cookie`, `source_ip` | `session` (for UDP), `table` (for HTTP/HTTPs) | This annotation is used to specify the default stickiness for Linode NodeBalancer |
| `port-*` | json object | | Specifies port specific NodeBalancer configuration. See [Port Configuration](#port-specific-configuration) |
| `check-type` | `none`, `connection`, `http`, `http_body` | `none` for UDP, else `connection` | The type of health check to perform against back-ends. See [Health Checks](loadbalancer.md#health-checks) |
| `check-path` | string | | The URL path to check on each back-end during health checks |
| `check-body` | string | | Text which must be present in the response body to pass the health check |
| `check-interval` | int | `5` | Duration, in seconds, to wait between health checks |
| `check-timeout` | int (1-30) | `3` | Duration, in seconds, to wait for a health check to succeed |
| `check-attempts` | int (1-30) | `2` | Number of health check failures necessary to remove a back-end |
| `check-passive` | bool | `false` | When `true`, `5xx` status codes will cause the health check to fail |
| `udp-check-port` | int | `80` | Specifies health check port for UDP nodebalancer |
| `preserve` | bool | `false` | When `true`, deleting a `LoadBalancer` service does not delete the underlying NodeBalancer |
| `nodebalancer-id` | int | | The ID of the NodeBalancer to front the service |
| `hostname-only-ingress` | bool | `false` | When `true`, the LoadBalancerStatus will only contain the Hostname |
| `tags` | string | | A comma separated list of tags to be applied to the NodeBalancer instance |
| `firewall-id` | int | | An existing Cloud Firewall ID to be attached to the NodeBalancer instance. See [Firewall Setup](firewall.md) |
| `firewall-acl` | string | | The Firewall rules to be applied to the NodeBalancer. See [Firewall Configuration](#firewall-configuration) |
| `nodebalancer-type` | string | | The type of NodeBalancer to create (options: common, premium). See [NodeBalancer Types](#nodebalancer-type) |
| `enable-ipv6-ingress` | bool | `false` | When `true`, both IPv4 and IPv6 addresses will be included in the LoadBalancerStatus ingress |
| `backend-ipv4-range` | string | | The IPv4 range from VPC subnet to be applied to the NodeBalancer backend. See [Nodebalancer VPC Configuration](#nodebalancer-vpc-configuration) |
| `backend-vpc-name` | string | | VPC which is connected to the NodeBalancer backend. See [Nodebalancer VPC Configuration](#nodebalancer-vpc-configuration) |
| `backend-subnet-name` | string | | Subnet within VPC which is connected to the NodeBalancer backend. See [Nodebalancer VPC Configuration](#nodebalancer-vpc-configuration) |

### Port Specific Configuration

The `port-*` annotation allows per-port configuration, encoded in JSON. For detailed examples, see [LoadBalancer SSL/TLS Setup](loadbalancer.md#ssltls-configuration).

```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "protocol": "https",
        "tls-secret-name": "my-tls-secret",
        "proxy-protocol": "v2",
        "algorithm": "leastconn",
        "stickiness": "http_cookie"
      }
```

Available port options:
- `protocol`: Protocol for this port (tcp, http, https)
- `tls-secret-name`: Name of TLS secret for HTTPS. The secret type should be `kubernetes.io/tls`
- `proxy-protocol`: Proxy protocol version for this port
- `algorithm`: Algorithm for this port
- `stickiness`: Stickiness for this port
- `udp-check-port`: UDP health check port for this port

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

### NodeBalancer Type
Linode supports nodebalancers of different types: common and premium. By default, nodebalancers of type common are provisioned. If an account is allowed to provision premium nodebalancers and one wants to use them, it can be achieved by specifying the annotation:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-type: premium
```

### Nodebalancer VPC Configuration
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-backend-ipv4-range: "10.100.0.0/30"
    service.beta.kubernetes.io/linode-loadbalancer-vpc-name: "vpc1"
    service.beta.kubernetes.io/linode-loadbalancer-subnet-name: "subnet1"
```

### Service with IPv6 Address
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-enable-ipv6-ingress: "true"
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
