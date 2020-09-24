# Kubernetes Cloud Controller Manager for Linode

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-cloud-controller-manager)](https://goreportcard.com/report/github.com/linode/linode-cloud-controller-manager)
[![Build Status](https://travis-ci.com/linode/linode-cloud-controller-manager.svg?branch=master)](https://travis-ci.com/linode/linode-cloud-controller-manager)
[![Coverage Status](https://coveralls.io/repos/github/linode/linode-cloud-controller-manager/badge.svg?branch=master)](https://coveralls.io/github/linode/linode-cloud-controller-manager?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-cloud-controller-manager.svg)](https://hub.docker.com/r/linode/linode-cloud-controller-manager/)
<!-- [![Slack](http://slack.kubernetes.io/badge.svg)](http://slack.kubernetes.io/#linode) -->
[![Twitter](https://img.shields.io/twitter/follow/linode.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=linode)

## The purpose of the CCM

The Linode Cloud Controller Manager (CCM) creates a fully supported
Kubernetes experience on Linode.

* Load balancers, Linode NodeBalancers, are automatically deployed when a
[Kubernetes Service of type "LoadBalancer"](https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer) is deployed. This is the most
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

## Usage

### LoadBalancer Services

Kubernetes Services of type `LoadBalancer` will be served through a [Linode NodeBalancer](https://www.linode.com/nodebalancers) which the Cloud Controller Manager will provision on demand.  For general feature and usage notes, refer to the [Getting Started with Linode NodeBalancers](https://www.linode.com/docs/platform/nodebalancer/getting-started-with-nodebalancers/) guide.

#### Annotations

The Linode CCM accepts several annotations which affect the properties of the underlying NodeBalancer deployment.

All of the service annotation names listed below have been shortened for readability.  Each annotation **MUST** be prefixed with `service.beta.kubernetes.io/linode-loadbalancer-`.  The values, such as `http`, are case-sensitive.

Annotation (Suffix) | Values | Default | Description
---|---|---|---
`throttle` | `0`-`20` (`0` to disable) | `20` | Client Connection Throttle, which limits the number of subsequent new connections per second from the same client IP
`default-protocol` | `tcp`, `http`, `https` | `tcp` | This annotation is used to specify the default protocol for Linode NodeBalancer.
`proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer
`port-*` | json (e.g. `{ "tls-secret-name": "prod-app-tls", "protocol": "https"}`) | | Specifies the secret and protocol for a port corresponding secrets. The secret type should be `kubernetes.io/tls`. `*` is the port being configured, e.g. `linode-loadbalancer-port-443`
`check-type` | `none`, `connection`, `http`, `http_body` | | The type of health check to perform against back-ends to ensure they are serving requests
`check-path` | string | | The URL path to check on each back-end during health checks
`check-body` | string | | Text which must be present in the response body to pass the NodeBalancer health check
`check-interval` | int | | Duration, in seconds, to wait between health checks
`check-timeout` | int (1-30) | | Duration, in seconds, to wait for a health check to succeed before considering it a failure
`check-attempts` | int (1-30) | | Number of health check failures necessary to remove a back-end from the service
`check-passive` | [bool](#annotation-bool-values) | `false` | When `true`, `5xx` status codes will cause the health check to fail
`preserve` | [bool](#annotation-bool-values) | `false` | When `true`, deleting a `LoadBalancer` service does not delete the underlying NodeBalancer. This will also prevent deletion of the former LoadBalancer when another one is specified with the `nodebalancer-id` annotation.
`nodebalancer-id` | string | | The ID of the NodeBalancer to front the service. When not specified, a new NodeBalancer will be created. This can be configured on service creation or patching

#### Deprecated Annotations

These annotations are deprecated, and will be removed Q3 2020.

Annotation (Suffix) | Values | Default | Description
---|---|---|---
`protocol` | `tcp`, `http`, `https` | `tcp` | This annotation is used to specify the default protocol for Linode NodeBalancer. For ports specified in the `linode-loadbalancer-tls-ports` annotation, this protocol is overwritten to `https`
`tls` | json array (e.g. `[ { "tls-secret-name": "prod-app-tls", "port": 443}, {"tls-secret-name": "dev-app-tls", "port": 8443} ]`) | | Specifies TLS ports with their corresponding secrets, the secret type should be `kubernetes.io/tls

#### Annotation bool values

For annotations with bool value types, `"1"`, `"t"`,  `"T"`, `"True"`, `"true"` and `"True"` are valid string representations of `true`. Any other values will be interpreted as false. For more details, see [strconv.ParseBool](https://golang.org/pkg/strconv/#ParseBool).

#### Example usage

```yaml
kind: Service
apiVersion: v1
metadata:
  name: https-lb
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-throttle: "4"
    service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
    service.beta.kubernetes.io/linode-loadbalancer-port-443: |
      {
        "tls-secret-name": "example-secret",
        "protocol": "https"
      }
spec:
  type: LoadBalancer
  selector:
    app: nginx-https-example
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
    - name: https
      protocol: TCP
      port: 443
      targetPort: https

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-https-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx-https-example
  template:
    metadata:
      labels:
        app: nginx-https-example
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
          - name: http
            containerPort: 80
            protocol: TCP
          - name: https
            containerPort: 80
            protocol: TCP

```

See more in the [examples directory](examples)

## Why `stickiness` and `algorithm` annotations don't exit

As kube-proxy will simply double-hop the traffic to a random backend Pod anyway, it doesn't matter which backend Node traffic is forwarded-to for the sake of session stickiness.
So these annotations are not necessary to implement session stickiness. 

## How to use sessionAffinity

In Kubernetes, sessionAffinity refers to a mechanism that allows a client always to be redirected to the same pod when the client hits a service.

To enable sessionAffinity `service.spec.sessionAffinity` field must be set to `ClientIP` as the following service yaml:

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

The max session sticky time can be set by setting the field `service.spec.sessionAffinityConfig.clientIP.timeoutSeconds` as below:

```yaml
sessionAffinityConfig:
  clientIP:
    timeoutSeconds: 100
```

## Generating a Manifest for Deployment

Use the script located at `./deploy/generate-manifest.sh` to generate a self-contained deployment manifest for the Linode CCM. Two arguments are required.

The first argument must be a Linode APIv4 Personal Access Token with all permissions.
(https://cloud.linode.com/profile/tokens)

The second argument must be a Linode region.
(https://api.linode.com/v4/regions)

Example:

```sh
./deploy/generate-manifest.sh $LINODE_API_TOKEN us-east
```

This will create a file `ccm-linode.yaml` which you can use to deploy the CCM.

`kubectl apply -f ccm-linode.yaml`

Note: Your kubelets, controller-manager, and apiserver must be started with `--cloud-provider=external` as noted in the following documentation.

### Upstream Documentation Including Deployment Instructions

[Kubernetes Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/).


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
go get github.com/linode/linode-cloud-controller-manager
cd $(go env GOPATH)/src/github.com/linode/linode-cloud-controller-manager
```

#### Install Dev tools

To install various dev tools for Pharm Controller Manager, run the following command:

```bash
./hack/builddeps.sh
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
dep ensure
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
docker run -ti linode/linode-cloud-controller-manager:canary
```

## Contribution Guidelines

Want to improve the linode-cloud-controller-manager? Please start [here](.github/CONTRIBUTING.md).

## Join the Kubernetes Community

For general help or discussion, join us in #linode on the [Kubernetes Slack](https://kubernetes.slack.com/messages/CD4B15LUR/details/). To sign up, use the [Kubernetes Slack inviter](http://slack.kubernetes.io/).
