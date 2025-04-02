# Kubernetes Cloud Controller Manager for Linode

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-cloud-controller-manager)](https://goreportcard.com/report/github.com/linode/linode-cloud-controller-manager)
[![Continuous Integration](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/linode/linode-cloud-controller-manager/graph/badge.svg?token=GSRnqHUmCk)](https://codecov.io/gh/linode/linode-cloud-controller-manager)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-cloud-controller-manager.svg)](https://hub.docker.com/r/linode/linode-cloud-controller-manager/)
[![Twitter](https://img.shields.io/twitter/follow/linode.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=linode)

## Overview

The Linode Cloud Controller Manager (CCM) is a crucial component that integrates Kubernetes with Linode's infrastructure services. It implements the cloud-controller-manager binary, running cloud-specific control loops that are essential for cluster operation.

A Cloud Controller Manager (CCM) is a Kubernetes control plane component that embeds cloud-specific control logic. It lets you link your cluster to your cloud provider's API, separating out the components that interact with that cloud platform from components that only interact with your cluster.

### Core Components

#### Node Controller
- Initializes node configuration with Linode-specific information
  - Sets node addresses (public/private IPs)
  - Labels nodes with region/zone information
  - Configures node hostnames
- Monitors node health and lifecycle
  - Detects node termination
  - Updates node status
  - Manages node cleanup

#### Service Controller
- Manages LoadBalancer service implementations using Linode NodeBalancers
  - Creates and configures NodeBalancers
  - Updates backend pools
  - Manages SSL/TLS certificates
- Handles automatic provisioning and configuration
  - Health checks
  - Session affinity
  - Protocol configuration
- Supports multiple load balancing approaches
  - Traditional NodeBalancer deployment
  - BGP-based IP sharing for cost optimization
  - Custom firewall rules and security configurations

#### Route Controller
- Manages VPC and private network integration
  - Configures routes for pod CIDR ranges
  - Handles cross-node pod communication
- Ensures proper network connectivity
  - Sets up pod-to-pod networking
  - Manages network policies
  - Configures network routes for optimal communication

#### NodeIPAM Controller
- Manages and configures pod CIDRs to nodes

## Requirements

- Kubernetes 1.22+
- Kubelets, controller-manager, and apiserver with `--cloud-provider=external`
- Linode APIv4 Token
- Supported Linode region

## Documentation

### Quick Start
- [Getting Started Guide](docs/getting-started/README.md) - Start here for installation and setup
  - [Overview](docs/getting-started/overview.md) - Learn about CCM basics
  - [Requirements](docs/getting-started/requirements.md) - Check prerequisites
  - [Installation](docs/getting-started/installation.md) - Install the CCM
    - [Helm Installation](docs/getting-started/helm-installation.md) - Install using Helm
    - [Manual Installation](docs/getting-started/manual-installation.md) - Manual setup instructions
  - [Verification](docs/getting-started/verification.md) - Verify your installation
  - [Troubleshooting](docs/getting-started/troubleshooting.md) - Common issues and solutions

### Configuration
- [Configuration Guide](docs/configuration/README.md) - Detailed configuration options
  - [LoadBalancer Services](docs/configuration/loadbalancer.md)
  - [Service Annotations](docs/configuration/annotations.md)
  - [Node Configuration](docs/configuration/nodes.md)
  - [Environment Variables](docs/configuration/environment.md)
  - [Firewall Setup](docs/configuration/firewall.md)
  - [Route Configuration](docs/configuration/routes.md)
  - [Session Affinity](docs/configuration/session-affinity.md)
  - [NodeIPAM Configuration](docs/configuration/nodeipam.md)

### Examples and Development
- [Examples](docs/examples/README.md) - Real-world usage examples
  - [Basic Services](docs/examples/basic.md)
  - [Advanced Configuration](docs/examples/advanced.md)
- [Development Guide](docs/development/README.md) - Contributing to CCM

## Getting Help

### Community Support

For general help or discussion, join us in #linode on the [Kubernetes Slack](https://kubernetes.slack.com/messages/CD4B15LUR/details/). 

To sign up for Kubernetes Slack, use the [Kubernetes Slack inviter](http://slack.kubernetes.io/).

### Issue Tracking

If you've found a bug or want to request a feature:
- Check the [GitHub Issues](https://github.com/linode/linode-cloud-controller-manager/issues)
- Submit a [Pull Request](https://github.com/linode/linode-cloud-controller-manager/pulls)

### Additional Resources

- [Official Linode Documentation](https://www.linode.com/docs/)
- [Kubernetes Cloud Controller Manager Documentation](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)
- [API Documentation](https://www.linode.com/docs/api)

## Contributing

Want to improve the Linode Cloud Controller Manager? Please see our [contributing guidelines](.github/CONTRIBUTING.md).
