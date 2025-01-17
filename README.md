# Kubernetes Cloud Controller Manager for Linode

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-cloud-controller-manager)](https://goreportcard.com/report/github.com/linode/linode-cloud-controller-manager)
[![Continuous Integration](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/ci.yml/badge.svg)](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/linode/linode-cloud-controller-manager/graph/badge.svg?token=GSRnqHUmCk)](https://codecov.io/gh/linode/linode-cloud-controller-manager)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-cloud-controller-manager.svg)](https://hub.docker.com/r/linode/linode-cloud-controller-manager/)
[![Twitter](https://img.shields.io/twitter/follow/linode.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=linode)

## Overview

The Linode Cloud Controller Manager (CCM) integrates Kubernetes with Linode's infrastructure services. It implements cloud-specific control loops essential for cluster operation. For detailed documentation, see the [docs](docs/src/introduction.md).

### Core Components

- **Node Controller**: Manages node lifecycle and configuration
- **Service Controller**: Handles LoadBalancer implementations using NodeBalancers
- **Route Controller**: Configures network routes for pod communication

### Key Features

- Automatic LoadBalancer provisioning using Linode NodeBalancers
- Node management and lifecycle operations
- Network route configuration for VPC environments
- Firewall management for services

## Requirements

- Kubernetes 1.9+
- Kubelets, controller-manager, and apiserver with `--cloud-provider=external`
- Linode APIv4 Token
- Supported Linode region

## Quick Links

- [Getting Started](docs/src/getting-started/README.md)
  - [Overview](docs/src/getting-started/overview.md)
  - [Requirements](docs/src/getting-started/requirements.md)
  - [Installation](docs/src/getting-started/installation.md)
    - [Helm Installation](docs/src/getting-started/helm-installation.md)
    - [Manual Installation](docs/src/getting-started/manual-installation.md)
  - [Verification](docs/src/getting-started/verification.md)
  - [Troubleshooting](docs/src/getting-started/troubleshooting.md)

- [Configuration Guide](docs/src/configuration/README.md)
  - [LoadBalancer Services](docs/src/configuration/loadbalancer.md)
  - [Service Annotations](docs/src/configuration/annotations.md)
  - [Node Configuration](docs/src/configuration/nodes.md)
  - [Environment Variables](docs/src/configuration/environment.md)
  - [Firewall Setup](docs/src/configuration/firewall.md)
  - [Route Configuration](docs/src/configuration/routes.md)
  - [Session Affinity](docs/src/configuration/session-affinity.md)

- [Examples](docs/src/examples/README.md)
  - [Basic Services](docs/src/examples/basic.md)
  - [Advanced Configuration](docs/src/examples/advanced.md)

- [Development Guide](docs/src/development/README.md)

- [Getting Help](docs/src/help.md)

## Getting Help

For support and development discussions, join us in #linode on the [Kubernetes Slack](https://kubernetes.slack.com/messages/CD4B15LUR/details/). To sign up, use the [Kubernetes Slack inviter](http://slack.kubernetes.io/).

## Contributing

Want to improve the Linode Cloud Controller Manager? Please see our [contributing guidelines](.github/CONTRIBUTING.md).
