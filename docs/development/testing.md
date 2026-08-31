---
layout: default
parent: Development Guide
nav_order: 1
---

# Testing

This guide describes the public test workflows for the Linode Cloud Controller Manager (CCM). Use a disposable Linode account or project and a dedicated test cluster. The end-to-end tests create and delete Kubernetes and Linode resources, including NodeBalancers, Cloud Firewalls, and reserved IPs.

## Prerequisites

Install the repository toolchain from the repository root:

```bash
mise install
```

The end-to-end workflows require a Linode API token. Store it in an environment variable or a local secret manager. **Do not commit tokens, kubeconfigs, or generated cluster manifests.**

At the very least, the token needs permission to manage resources exercised by the test suite:

- Linodes
- NodeBalancers
- IPs
- Firewalls
- VPC permissions are also needed when running tests that use VPC-backed NodeBalancers.

## Unit Tests

Run the unit test suite locally:

```bash
mise run test
```

This command formats the Go source, regenerates generated code, and writes the coverage report to `coverage.out`. It does not call the Linode API or require a cluster.

To run a focused Go test while using the pinned toolchain:

```bash
mise exec -- go test ./cloud/linode -run '^Test_getPortConfig$'
```

## End-to-End Tests with CAPL

The full end-to-end suite uses a local Kind management cluster and [Cluster API Provider Linode (CAPL)](https://github.com/linode/cluster-api-provider-linode) to provision temporary workload clusters. This is the same workflow used by the public CI configuration.

You need a Docker-compatible daemon and permission to push a CCM image to a container registry that the workload-cluster nodes can pull from. Set a unique image tag so your run does not overwrite another image:

```bash
export LINODE_TOKEN='replace-with-your-token'
export KO_DOCKER_REPO='registry.example.com/your-namespace/linode-cloud-controller-manager'
export IMAGE_TAGS="e2e-$(git rev-parse --short HEAD)"
export LINODE_CONTROL_PLANE_MACHINE_TYPE="g6-standard-2"
export LINODE_MACHINE_TYPE="g6-standard-2"
```

The Make targets use their default `LINODE_REGION`, `LINODE_URL`, `CLUSTER_NAME`, and `E2E_RESERVED_IP_TAG` values unless you override them. See [End-to-End Tests with LKE](#end-to-end-tests-with-lke) for the defaults and override syntax.

Build and publish the test image, provision the management and workload clusters, then run the suite:

```bash
mise run ko-publish
mise run mgmt-and-capl-cluster
mise run e2e-test
mise run e2e-test-subnet
```

Run cleanup even when a test fails:

```bash
mise run cleanup-cluster
```

`mise run e2e-test` includes the IPv6 backend test slice. It is intended for the CAPL-provisioned clusters; do not use it against an existing LKE cluster.

## End-to-End Tests with LKE

The scenarios labelled `lke` can run against an existing LKE cluster. Use a disposable LKE cluster that already runs the CCM image you want to validate. These tests create namespaces and LoadBalancer Services, and some scenarios create or modify NodeBalancers, Cloud Firewalls, and reserved IPs in the cluster's region.

Download the kubeconfig for the disposable cluster and set the two required inputs:

```bash
export KUBECONFIG="$HOME/.kube/ccm-lke-test.yaml"
export LINODE_TOKEN='replace-with-your-token'
```

Confirm that the selected kubeconfig points at the disposable cluster and that the CCM is ready:

```bash
kubectl config current-context
kubectl get nodes
kubectl -n kube-system rollout status daemonset/ccm-linode --timeout=10m
```

If you are testing a new image, update the `ccm-linode` DaemonSet with an image available to every LKE node, then wait for the rollout before running tests:

```bash
kubectl -n kube-system set image daemonset/ccm-linode ccm-linode=registry.example.com/your-namespace/linode-cloud-controller-manager:your-tag
kubectl -n kube-system rollout status daemonset/ccm-linode --timeout=10m
```

Run every LKE-compatible scenario:

```bash
LINODE_URL="${LINODE_URL:-https://api.linode.com}" \
LINODE_REGION="${LINODE_REGION:-us-lax}" \
REGION="${LINODE_REGION:-us-lax}" \
CLUSTER_NAME="${CLUSTER_NAME:-ccm-$(git rev-parse --short HEAD)}" \
E2E_RESERVED_IP_TAG="${E2E_RESERVED_IP_TAG:-ccm-e2e-$(git rev-parse --short HEAD)}" \
mise exec -- chainsaw test e2e/test --parallel 2 --selector lke
```

To investigate a single scenario, pass its directory instead. For example:

```bash
mise exec -- chainsaw test e2e/test/lb-simple
```

The direct Chainsaw command supplies the same defaults as the CAPL Make targets. Set an environment variable before running it only to override a value:

| Variable | Default | Purpose |
| --- | --- | --- |
| `LINODE_REGION` | `us-lax` | Region for resources created directly by test scenarios. Set it to the LKE cluster's region. |
| `LINODE_URL` | `https://api.linode.com` | Linode API endpoint. Change only when using a different endpoint. |
| `CLUSTER_NAME` | `ccm-<git-short-sha>` | Suffix used in names of test-created resources. Use a unique value for concurrent runs or shared accounts. |
| `E2E_RESERVED_IP_TAG` | `ccm-e2e-<git-short-sha>` | Tag used to identify reserved IPs created by the suite. Use a unique value for concurrent runs or shared accounts. |

For example, to test an LKE cluster in another region with unique resource identifiers:

```bash
export LINODE_REGION='us-east'
export CLUSTER_NAME="ccm-lke-$(date +%s)"
export E2E_RESERVED_IP_TAG="$CLUSTER_NAME"
```

Then run the full command above.

Chainsaw cleans up Kubernetes resources after each test. If a run is interrupted, sweep only reserved IPs with the tag used for that run:

```bash
E2E_RESERVED_IP_TAG="${E2E_RESERVED_IP_TAG:-ccm-e2e-$(git rev-parse --short HEAD)}" \
LINODE_TOKEN="$LINODE_TOKEN" \
LINODE_URL="${LINODE_URL:-https://api.linode.com}" \
./e2e/test/scripts/cleanup-reserved-ips.sh sweep
```

Review the remaining test namespaces, NodeBalancers, and Cloud Firewalls before deleting the disposable cluster. Do not reuse a shared production cluster or a shared reserved-IP tag for these tests.

## Test Selection

The E2E manifests use labels to select compatible scenarios:

| Selector | Intended environment |
| --- | --- |
| `lke` | Existing LKE cluster |
| `all` | CAPL-provisioned primary workload cluster |
| `ipv6-backends` | CAPL-provisioned IPv6 workload cluster |

Some tests require a CAPL management cluster to add nodes or configure routes. They are intentionally excluded from the `lke` selector.
