# Kubernetes Cloud Controller Manager for Linode

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-cloud-controller-manager)](https://goreportcard.com/report/github.com/linode/linode-cloud-controller-manager)
[![Test](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/test.yml/badge.svg)](https://github.com/linode/linode-cloud-controller-manager/actions/workflows/test.yml)
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

All of the Service annotation names listed below have been shortened for readability.  The values, such as `http`, are case-sensitive.

Each *Service* annotation **MUST** be prefixed with:<br />
**`service.beta.kubernetes.io/linode-loadbalancer-`**

Annotation (Suffix) | Values | Default | Description
---|---|---|---
`throttle` | `0`-`20` (`0` to disable) | `0` | Client Connection Throttle, which limits the number of subsequent new connections per second from the same client IP
`default-protocol` | `tcp`, `http`, `https` | `tcp` | This annotation is used to specify the default protocol for Linode NodeBalancer.
`default-proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer.
`port-*` | json (e.g. `{ "tls-secret-name": "prod-app-tls", "protocol": "https", "proxy-protocol": "v2"}`) | | Specifies port specific NodeBalancer configuration. See [Port Specific Configuration](#port-specific-configuration). `*` is the port being configured, e.g. `linode-loadbalancer-port-443`
`check-type` | `none`, `connection`, `http`, `http_body` | | The type of health check to perform against back-ends to ensure they are serving requests
`check-path` | string | | The URL path to check on each back-end during health checks
`check-body` | string | | Text which must be present in the response body to pass the NodeBalancer health check
`check-interval` | int | | Duration, in seconds, to wait between health checks
`check-timeout` | int (1-30) | | Duration, in seconds, to wait for a health check to succeed before considering it a failure
`check-attempts` | int (1-30) | | Number of health check failures necessary to remove a back-end from the service
`check-passive` | [bool](#annotation-bool-values) | `false` | When `true`, `5xx` status codes will cause the health check to fail
`preserve` | [bool](#annotation-bool-values) | `false` | When `true`, deleting a `LoadBalancer` service does not delete the underlying NodeBalancer. This will also prevent deletion of the former LoadBalancer when another one is specified with the `nodebalancer-id` annotation.
`nodebalancer-id` | string | | The ID of the NodeBalancer to front the service. When not specified, a new NodeBalancer will be created. This can be configured on service creation or patching
`hostname-only-ingress` | [bool](#annotation-bool-values) | `false` | When `true`, the LoadBalancerStatus for the service will only contain the Hostname. This is useful for bypassing kube-proxy's rerouting of in-cluster requests originally intended for the external LoadBalancer to the service's constituent pod IPs.
`tags` | string | | A comma seperated list of tags to be applied to the createad NodeBalancer instance
`firewall-id` | string | | An existing Cloud Firewall ID to be attached to the NodeBalancer instance. See [Firewalls](#firewalls).
`firewall-acl` | string | | The Firewall rules to be applied to the NodeBalancer. Adding this annotation creates a new CCM managed Linode CloudFirewall instance. See [Firewalls](#firewalls).

#### Deprecated Annotations
These annotations are deprecated, and will be removed in a future release.

Annotation (Suffix) | Values | Default | Description | Scheduled Removal
---|---|---|---|---
`proxy-protcol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer | Q4 2021

#### Annotation bool values
For annotations with bool value types, `"1"`, `"t"`,  `"T"`, `"True"`, `"true"` and `"True"` are valid string representations of `true`. Any other values will be interpreted as false. For more details, see [strconv.ParseBool](https://golang.org/pkg/strconv/#ParseBool).

#### Port Specific Configuration
These configuration options can be specified via the `port-*` annotation, encoded in JSON.

Key | Values | Default | Description
---|---|---|---
`protocol` | `tcp`, `http`, `https` | `tcp` | Specifies protocol of the NodeBalancer port. Overwrites `default-protocol`.
`proxy-protocol` | `none`, `v1`, `v2` | `none` | Specifies whether to use a version of Proxy Protocol on the underlying NodeBalancer. Overwrites `default-proxy-protocol`.
`tls-secret-name` | string | | Specifies a secret to use for TLS. The secret type should be `kubernetes.io/tls`.

#### Firewalls
Firewall rules can be applied to the CCM Managed NodeBalancers in two distinct ways.

##### CCM Managed Firewall
To use this feature, ensure that the linode api token used with the ccm has the `add_firewalls` grant. 

The CCM accepts firewall ACLs in json form. The ACL can either be an `allowList` or a `denyList`. Supplying both is not supported. Supplying neither is not supported. The `allowList` sets up a CloudFirewall that `ACCEPT`s traffic only from the specified IPs/CIDRs and `DROP`s everything else. The `denyList` sets up a CloudFirewall that `DROP`s traffic only from the specified IPs/CIDRs and `ACCEPT`s everything else. Ports are automatically inferred from the service configuration. 

See [Firewall rules](https://www.linode.com/docs/api/networking/#firewall-create__request-body-schema) for more details on how to specify the IPs/CIDRs

Example usage of an ACL to allow traffic from a specific set of addresses

```yaml
kind: Service
apiVersion: v1
metadata:
  name: https-lb
  annotations:
    service.beta.kubernetes.io/linode-loadbalancer-firewall-acl: |
      {
        "allowList": {
          "ipv4": ["192.166.0.0/16", "172.23.41.0/24"],
          "ipv6": ["2001:DB8::/128"]
        },
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
```


##### User Managed Firewall
Users can create CloudFirewall instances, supply their own rules and attach them to the NodeBalancer. To do so, set the
`service.beta.kubernetes.io/linode-loadbalancer-firewall-id` annotation to the ID of the cloud firewall. The CCM does not manage the lifecycle of the CloudFirewall Instance in this case. Users are responsible for ensuring the policies are correct.

**Note**<br/>
If the user supplies a firewall-id, and later switches to using an ACL, the CCM will take over the CloudFirewall Instance. To avoid this, delete the service, and re-create it so the original CloudFirewall is left undisturbed.

#### Routes
When running k8s clusters within VPC, node specific podCIDRs need to be allowed on the VPC interface. Linode CCM comes with route-controller functionality which can be enabled for automatically adding/deleting routes on VPC interfaces. When installing CCM with helm, make sure to specify routeController settings.

##### Example usage in values.yaml
```yaml
routeController:
  vpcName: <name of VPC>
  clusterCIDR: 10.0.0.0/8
  configureCloudRoutes: true
```

### Nodes
Kubernetes Nodes can be configured with the following annotations.

Each *Node* annotation **MUST** be prefixed with:<br />
**`node.k8s.linode.com/`**

Key | Values | Default | Description
---|---|---|---
`private-ip` | `IPv4` | `none` | Specifies the Linode Private IP overriding default detection of the Node InternalIP.<br />When using a [VLAN] or [VPC], the Node InternalIP may not be a Linode Private IP as [required for NodeBalancers] and should be specified.


[required for NodeBalancers]: https://www.linode.com/docs/api/nodebalancers/#nodebalancer-create__request-body-schema
[VLAN]: https://www.linode.com/products/vlan/
[VPC]: https://www.linode.com/blog/linode/new-betas-coming-to-green-light/

### Example usage
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

## Why `stickiness` and `algorithm` annotations don't exist
As kube-proxy will simply double-hop the traffic to a random backend Pod anyway, it doesn't matter which backend Node traffic is forwarded-to for the sake of session stickiness.
These annotations are not necessary to implement session stickiness, as kube-proxy will simply double-hop the packets to a random backend Pod. It would not make a difference to set a backend Node that would receive the network traffic in an attempt to set session stickiness.

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

## Deployment Through Helm Chart
LINODE_API_TOKEN must be a Linode APIv4 [Personal Access Token](https://cloud.linode.com/profile/tokens) with all permissions.

REGION must be a Linode [region](https://api.linode.com/v4/regions).
### Install the ccm-linode repo
```shell
helm repo add ccm-linode https://linode.github.io/linode-cloud-controller-manager/ 
helm repo update ccm-linode
```

### To deploy ccm-linode. Run the following command:

```sh
export VERSION=v0.3.22
export LINODE_API_TOKEN=<linodeapitoken>
export REGION=<linoderegion>
helm install ccm-linode --set apiToken=$LINODE_API_TOKEN,region=$REGION ccm-linode/ccm-linode
```
_See [helm install](https://helm.sh/docs/helm/helm_install/) for command documentation._

### To uninstall ccm-linode from kubernetes cluster. Run the following command:
```sh
helm uninstall ccm-linode
```
_See [helm uninstall](https://helm.sh/docs/helm/helm_uninstall/) for command documentation._

### To upgrade when new changes are made to the helm chart. Run the following command:
```sh
export VERSION=v0.3.22
export LINODE_API_TOKEN=<linodeapitoken>
export REGION=<linoderegion>

helm upgrade ccm-linode --install --set apiToken=$LINODE_API_TOKEN,region=$REGION ccm-linode/ccm-linode
```
_See [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/) for command documentation._

### Configurations
There are other variables that can be set to a different value. For list of all the modifiable variables/values, take a look at './deploy/chart/values.yaml'.

Values can be set/overrided by using the '--set var=value,...' flag or by passing in a custom-values.yaml using '-f custom-values.yaml'.

Recommendation: Use custom-values.yaml to override the variables to avoid any errors with template rendering

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
$ dist/linode-cloud-controller-manager
```

#### Dependency management
Linode Cloud Controller Manager uses [Go Modules](https://blog.golang.org/using-go-modules) to manage dependencies.
If you want to update/add dependencies, run:

```bash
go mod tidy
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
