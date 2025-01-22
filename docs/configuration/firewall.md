# Firewall Setup

## Overview

The CCM provides two methods for securing NodeBalancers with firewalls:
1. CCM-managed Cloud Firewalls (using `firewall-acl` annotation)
2. User-managed Cloud Firewalls (using `firewall-id` annotation)

## CCM-Managed Firewalls

### Configuration

Use the `firewall-acl` annotation to specify firewall rules. The rules should be provided as a JSON object with either an `allowList` or `denyList` (but not both).

#### Allow List Configuration
```yaml
apiVersion: v1
kind: Service
metadata:
  name: restricted-service
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-acl: |
      {
        "allowList": {
          "ipv4": ["192.168.0.0/16", "10.0.0.0/8"],
          "ipv6": ["2001:db8::/32"]
        }
      }
```

#### Deny List Configuration
```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-acl: |
      {
        "denyList": {
          "ipv4": ["203.0.113.0/24"],
          "ipv6": ["2001:db8:1234::/48"]
        }
      }
```

### Behavior
- Only one type of list (allow or deny) can be used per service
- Rules are automatically created and managed by the CCM
- Rules are updated when the annotation changes
- Firewall is deleted when the service is deleted (unless preserved)

## User-Managed Firewalls

### Configuration

1. Create a Cloud Firewall in Linode Cloud Manager
2. Attach it to the service using the `firewall-id` annotation:

```yaml
metadata:
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-id: "12345"
```

### Management
- User maintains full control over firewall rules
- Firewall persists after service deletion
- Manual updates required for rule changes

## Best Practices

1. **Rule Management**
   - Use descriptive rule labels
   - Document rule changes
   - Regular security audits

2. **IP Range Planning**
   - Plan CIDR ranges carefully
   - Document allowed/denied ranges
   - Consider future expansion

For more information:
- [Service Annotations](annotations.md#firewall-configuration)
- [LoadBalancer Configuration](loadbalancer.md)
- [Linode Cloud Firewall Documentation](https://www.linode.com/docs/products/networking/cloud-firewall/)
