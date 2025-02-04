# Requirements

Before installing the Linode Cloud Controller Manager, ensure your environment meets the following requirements.

## Kubernetes Cluster Requirements

### Version Compatibility
- Kubernetes version 1.22 or higher
- Kubernetes cluster running on Linode infrastructure

### Kubernetes Components Configuration
The following Kubernetes components must be started with the `--cloud-provider=external` flag:
- Kubelet
- Kube Controller Manager
- Kube API Server

## Linode Requirements

### API Token
You need a Linode APIv4 Personal Access Token with the following scopes:
- Linodes - Read/Write
- NodeBalancers - Read/Write
- IPs - Read/Write
- Volumes - Read/Write
- Firewalls - Read/Write (if using firewall features)
- VPCs - Read/Write (if using VPC features)
- VLANs - Read/Write (if using VLAN features)

To create a token:
1. Log into the [Linode Cloud Manager](https://cloud.linode.com)
2. Go to your profile
3. Select the "API Tokens" tab
4. Click "Create a Personal Access Token"
5. Select the required scopes
6. Set an expiry (optional)

### Region Support
Your cluster must be in a [supported Linode region](https://api.linode.com/v4/regions).

## Network Requirements

### Private Networking
- If using NodeBalancers, nodes must have private IP addresses
- VPC or VLAN configurations require additional network configuration

### Firewall Considerations
- Ensure required ports are open for Kubernetes components
- If using Cloud Firewalls, ensure the API token has firewall management permissions

## Resource Quotas
Ensure your Linode account has sufficient quota for:
- NodeBalancers (if using LoadBalancer services)
- Additional IP addresses (if using shared IP features)
- Cloud Firewalls (if using firewall features)
