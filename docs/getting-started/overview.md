# Overview

The Linode Cloud Controller Manager provides several key features that enable a fully supported Kubernetes experience on Linode infrastructure.

## Features

### LoadBalancer Services
- Automatic deployment and configuration of Linode NodeBalancers
- Support for HTTP, HTTPS, and TCP traffic
- SSL/TLS termination
- Custom health checks and session affinity

### Node Management
- Automatic configuration of node hostnames and network addresses
- Proper node state management for Linode shutdowns
- Region-based node annotation for failure domain scheduling

### Network Integration
- Support for private networking
- VPC and VLAN compatibility
- BGP-based IP sharing capabilities

### Security
- Integrated firewall management
- Support for TLS termination
- Custom security rules and ACLs

## When to Use CCM

The Linode CCM is essential when:
- Running Kubernetes clusters on Linode infrastructure
- Requiring automated load balancer provisioning
- Needing integrated cloud provider features
- Managing multi-node clusters with complex networking requirements 