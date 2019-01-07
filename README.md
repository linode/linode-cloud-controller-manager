# Kubernetes Cloud Controller Manager for Linode

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-cloud-controller-manager)](https://goreportcard.com/report/github.com/linode/linode-cloud-controller-manager)
[![Build Status](https://travis-ci.org/linode/linode-cloud-controller-manager.svg?branch=master)](https://travis-ci.org/linode/linode-cloud-controller-manager)
[![Coverage Status](https://coveralls.io/repos/github/linode/linode-cloud-controller-manager/badge.svg?branch=master)](https://coveralls.io/github/linode/linode-cloud-controller-manager?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-cloud-controller-manager.svg)](https://hub.docker.com/r/linode/linode-cloud-controller-manager/)
[![Slack](http://slack.kubernetes.io/badge.svg)](http://slack.kubernetes.io/#linode)
[![Twitter](https://img.shields.io/twitter/follow/linode.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=linode)

## What does it do?

The Linode Cloud Controller Manager (CCM) creates a fully supported
Kubernetes experience on Linode.

* Load balancers, Linode NodeBalancers, are automatically deployed when a
Kubernetes Service of type "LoadBalancer" is deployed. This is the most
reliable way to allow services running in your cluster to be reachable from
the Internet.
* Linode hostnames and network addresses (private/public IPs) are automatically
associated with their corresponding Kubernetes resources, forming the basis for 
a variety of Kubernetes features.
* Nodes resources are put into the correct state when Linodes are shut down, 
allowing pods to be appropriately rescheduled.
* Nodes are annotated with the Linode region, which is the basis for scheduling based on
failure domains.

## Kubernetes Supported Versions

Kubernetes 1.9+

## Upstream Documentation Including Deployment Instructions

[Kubernetes Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/).

### Generating a Manifest for Deployment

Use the script located at `./hack/deploy/generate-manifest.sh` to generate a self-contained deployment manifest for the Linode CCM. Two arguments are required.

The first argument must be a Linode APIv4 Personal Access Token with all permissions.
(https://cloud.linode.com/profile/tokens)

The second argument must be a Linode region.
(https://api.linode.com/v4/regions)

Example:
```
$ ./generate-manifest.sh $LINODE_API_TOKEN us-east
```

This will create a file `ccm-linode.yaml` which you can use to deploy the CCM.

## Upstream Developer Documentation

[Developing a Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/).

## Development Guide

### Building the Linode Cloud Controller Manager

Some of the Linode Cloud Controller Manager development helper scripts rely
on a fairly up-to-date GNU tools environment, so most recent Linux distros
should work just fine out-of-the-box.

#### Setup Go

The Linode Cloud Controller Manager is written in Google's Go programming
language. Currently, the Linode Cloud Controller Manager is developed and
tested on **Go 1.8.3**. If you haven't set up a Go development environment,
please follow [these instructions](https://golang.org/doc/install) to
install Go.

On macOS, Homebrew has a nice package

```bash
brew install golang
```

#### Download Source

```bash
$ go get github.com/linode/linode-cloud-controller-manager
$ cd $(go env GOPATH)/src/github.com/linode/linode-cloud-controller-manager
```

#### Install Dev tools

To install various dev tools for Pharm Controller Manager, run the following command:

```bash
$ ./hack/builddeps.sh
```

#### Build Binary

Use the following Make targets to build and run a local binary

```bash
$ make build
$ make run
# You can also run the binary directly to pass additional args
$ dist/linode-cloud-controller-manager run
```

#### Dependency management

Linode Cloud Controller Manager uses [Dep](https://github.com/golang/dep) to
manage dependencies. Dependencies are already checked in the `vendor` folder.
If you want to update/add dependencies, run:
```bash
$ dep ensure
```

#### Building Docker images

To build and push a Docker image, use the following make targets. 

```bash
# Set the repo/image:tag with the TAG environment variable
# Then run the docker-build make target
$ IMG=linode/linode-cloud-controller-manager:canary make docker-build

# Push Image
$ IMG=linode/linode-cloud-controller-manager:canary make docker-push
```

Then, to run the image
```bash
$ docker run -ti linode/linode-cloud-controller-manager:canary
```

## Contribution Guidelines

Want to improve the linode-cloud-controller-manager? Please start [here](.github/CONTRIBUTING.md).

## Join the Kubernetes Community

For general help or discussion, join us in #linode on the [Kubernetes Slack](https://kubernetes.slack.com/messages/CD4B15LUR/details/). To sign up, use the [Kubernetes Slack inviter](http://slack.kubernetes.io/).
