# LoadBalancer Services Configuration

## Overview

The CCM supports two types of LoadBalancer implementations:
1. Linode NodeBalancers (default)
2. BGP-based IP sharing

For implementation examples, see [Basic Service Examples](../examples/basic.md#loadbalancer-services).

## NodeBalancer Implementation

When using NodeBalancers, the CCM automatically:
1. Creates and configures a NodeBalancer
2. Sets up backend nodes
3. Manages health checks
4. Handles SSL/TLS configuration

For more details, see [Linode NodeBalancer Documentation](https://www.linode.com/docs/products/networking/nodebalancers/).

### IPv6 Support

NodeBalancers support both IPv4 and IPv6 ingress addresses. By default, the CCM uses only IPv4 address for LoadBalancer services. 

You can enable IPv6 addresses globally for all services by setting the `enable-ipv6-for-loadbalancers` flag:

```yaml
spec:
  template:
    spec:
      containers:
        - name: ccm-linode
          args:
            - --enable-ipv6-for-loadbalancers=true
```

Alternatively, you can enable IPv6 addresses for individual services using the annotation:

```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-enable-ipv6-ingress: "true"
```

When IPv6 is enabled (either globally or per-service), both IPv4 and IPv6 addresses will be included in the service's LoadBalancer status.

### Basic Configuration

Create a LoadBalancer service:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  type: LoadBalancer
  ports:
    - port: 80
      targetPort: 8080
  selector:
    app: my-app
```

See [Advanced Configuration Examples](../examples/advanced.md#loadbalancer-services) for more complex setups.

### NodeBalancer Settings

#### Protocol Configuration
Available protocols:
- `tcp` (default)
- `http`
- `https`
- `udp`

Set the default protocol:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
```

See [Service Annotations](annotations.md#basic-configuration) for all protocol options.

### Health Checks

Configure health checks using annotations:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-check-type: "http"
    service.beta.kubernetes.io/linode-loadbalancer-check-path: "/healthz"
    service.beta.kubernetes.io/linode-loadbalancer-check-interval: "5"
    service.beta.kubernetes.io/linode-loadbalancer-check-timeout: "3"
    service.beta.kubernetes.io/linode-loadbalancer-check-attempts: "2"
```

Available check types:
- `none`: No health check
- `connection`: TCP connection check
- `http`: HTTP status check
- `http_body`: HTTP response body check

For more details, see [Health Check Configuration](annotations.md#health-check-configuration).

### SSL/TLS Configuration

1. Create a TLS secret:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-tls-secret
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
```

2. Reference in service annotation:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "protocol": "https",
        "tls-secret-name": "my-tls-secret"
      }
```

### Connection Throttling

Limit connections from the same client IP:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-throttle: "5"
```

### Proxy Protocol

Enable proxy protocol for client IP preservation:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-default-proxy-protocol: "v2"
```

## BGP-based IP Sharing Implementation

BGP-based IP sharing provides a more cost-effective solution for multiple LoadBalancer services. For detailed setup instructions, see [Cilium BGP Documentation](https://docs.cilium.io/en/stable/network/bgp-control-plane/bgp-control-plane/).

### Prerequisites
- [Cilium CNI](https://docs.cilium.io/en/stable/network/bgp-control-plane/bgp-control-plane/) with BGP control plane enabled
- Additional IP provisioning enabled on your account (contact [Linode Support](https://www.linode.com/support/))
- Nodes labeled for BGP peering

### Configuration

1. Enable BGP in CCM deployment:
```yaml
args:
  - --load-balancer-type=cilium-bgp
  - --bgp-node-selector=cilium-bgp-peering=true
  - --ip-holder-suffix=mycluster
```

2. Label nodes that should participate in BGP peering:
```bash
kubectl label node my-node cilium-bgp-peering=true
```

3. Create LoadBalancer services as normal - the CCM will automatically use BGP-based IP sharing instead of creating NodeBalancers.

### Environment Variables
- `BGP_CUSTOM_ID_MAP`: Use your own map instead of default region map for BGP
- `BGP_PEER_PREFIX`: Use your own BGP peer prefix instead of default one

For more details, see [Environment Variables](environment.md#network-configuration).

## Configuring NodeBalancers directly with VPC
NodeBalancers can be configured to have VPC specific ips configured as backend nodes. It requires:
1. VPC with a subnet and Linodes in VPC
2. Each NodeBalancer created within that VPC needs a free /30 or bigger subnet from the subnet to which Linodes are connected

Specify NodeBalancer backend ipv4 range when creating service:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-backend-ipv4-range: "10.100.0.0/30"
```

By default, CCM uses first VPC and Subnet name configured with it to attach NodeBalancers to that VPC subnet. To overwrite those, use:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-backend-ipv4-range: "10.100.0.4/30"
    service.beta.kubernetes.io/linode-loadbalancer-vpc-name: "vpc1"
    service.beta.kubernetes.io/linode-loadbalancer-subnet-name: "subnet1"
```

If CCM is started with `--nodebalancer-backend-ipv4-subnet` flag, then it will not allow provisioning of nodebalancer unless subnet specified in service annotation lie within the subnet specified using the flag. This is to prevent accidental overlap between nodebalancer backend ips and pod CIDRs.

## Advanced Configuration

### Using Existing NodeBalancers

Specify an existing NodeBalancer:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id: "12345"
```

### NodeBalancer Preservation

Prevent NodeBalancer deletion when service is deleted:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-preserve: "true"
```

### Port Configuration

Configure individual ports:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "protocol": "https",
        "tls-secret-name": "my-tls-secret",
        "proxy-protocol": "v2"
      }
```

### Tags

Add tags to NodeBalancer:
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-tags: "production,web-tier"
```

## Related Documentation

- [Service Annotations](annotations.md)
- [Firewall Configuration](firewall.md)
- [Session Affinity](session-affinity.md)
- [Environment Variables and Flags](environment.md)
- [Route Configuration](routes.md)
- [Linode NodeBalancer Documentation](https://www.linode.com/docs/products/networking/nodebalancers/)
- [Cilium BGP Documentation](https://docs.cilium.io/en/stable/network/bgp-control-plane/bgp-control-plane/)
- [Basic Service Examples](../examples/basic.md)
- [Advanced Configuration Examples](../examples/advanced.md)
