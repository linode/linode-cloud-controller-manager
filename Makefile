DEFAULT_KO_DOCKER_REPO  := docker.io/linode/linode-cloud-controller-manager
DEFAULT_IMAGE_TAGS      := canary

ifdef IMG
IMG_REPO_FROM_REF       := $(shell printf '%s\n' "$(IMG)" | sed 's/:[^:]*$$//')
IMG_TAG_FROM_REF        := $(shell printf '%s\n' "$(IMG)" | sed 's/^.*://')
endif

KO_DOCKER_REPO          ?= $(or $(IMG_REPO_FROM_REF),$(DEFAULT_KO_DOCKER_REPO))
IMAGE_TAGS              ?= $(or $(IMG_TAG_FROM_REF),$(DEFAULT_IMAGE_TAGS))
IMG                     ?= $(KO_DOCKER_REPO):$(IMAGE_TAGS)
RELEASE_DIR             ?= release
PLATFORM                ?= linux/amd64

#####################################################################
# Dev Setup
#####################################################################
CLUSTER_NAME            ?= ccm-$(shell git rev-parse --short HEAD)
SUBNET_CLUSTER_NAME     ?= subnet-testing-$(shell git rev-parse --short HEAD)
VPC_NAME                ?= $(CLUSTER_NAME)
MANIFEST_NAME           ?= capl-cluster-manifests
SUBNET_MANIFEST_NAME    ?= subnet-testing-manifests
IPV6_CLUSTER_NAME       ?= ipv6-$(shell git rev-parse --short HEAD)
IPV6_MANIFEST_NAME      ?= ipv6-manifests

# renovate: datasource=github-tags depName=kubernetes/kubernetes
K8S_VERSION             ?= "v1.36.2"

# renovate: datasource=github-tags depName=kubernetes-sigs/cluster-api
CAPI_VERSION            ?= "v1.13.3"

# renovate: datasource=github-tags depName=linode/cluster-api-provider-linode
CAPL_VERSION            ?= "v0.10.7"

CONTROLPLANE_NODES      ?= 1
WORKER_NODES            ?= 1
LINODE_FIREWALL_ENABLED ?= true
LINODE_REGION           ?= us-lax
LINODE_OS               ?= linode/ubuntu22.04
LINODE_URL              ?= https://api.linode.com
KUBECONFIG_PATH         ?= $(CURDIR)/test-cluster-kubeconfig.yaml
SUBNET_KUBECONFIG_PATH	?= $(CURDIR)/subnet-testing-kubeconfig.yaml
MGMT_KUBECONFIG_PATH    ?= $(CURDIR)/mgmt-cluster-kubeconfig.yaml
IPV6_KUBECONFIG_PATH    ?= $(CURDIR)/ipv6-kubeconfig.yaml

export GO111MODULE=on

.PHONY: all
all: build

.PHONY: clean
clean:
	@go clean .
	@rm -rf ./.tmp
	@rm -rf dist/*
	@rm -rf $(RELEASE_DIR)
	@rm -rf ./bin

.PHONY: codegen
codegen:
	go generate ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run -c .golangci.yml --fix

.PHONY: gosec
gosec: ## Run gosec against code.
	docker run --rm -v "$(PWD):/var/work:ro" -w /var/work securego/gosec:2.19.0 \
		-exclude-dir=bin -exclude-generated ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: test
# we say code is not worth testing unless it's formatted
test: fmt codegen
	go test -v -coverpkg=./sentry,./cloud/linode/client,./cloud/linode,./cloud/linode/utils,./cloud/linode/services,./cloud/nodeipam,./cloud/nodeipam/ipam -coverprofile ./coverage.out -cover ./sentry/... ./cloud/... $(TEST_ARGS)

.PHONY: build-linux
build-linux: codegen
	echo "cross compiling linode-cloud-controller-manager for linux/amd64" && \
		GOOS=linux GOARCH=amd64 \
		CGO_ENABLED=0 \
		go build -o dist/linode-cloud-controller-manager-linux-amd64 .

.PHONY: build
build: codegen
	echo "compiling linode-cloud-controller-manager" && \
		CGO_ENABLED=0 \
		go build -o dist/linode-cloud-controller-manager .

.PHONY: release
release:
	mkdir -p $(RELEASE_DIR)
	sed -e 's/appVersion: "latest"/appVersion: "$(IMAGE_VERSION)"/g' ./deploy/chart/Chart.yaml
	tar -czvf ./$(RELEASE_DIR)/helm-chart-$(IMAGE_VERSION).tgz -C ./deploy/chart .

.PHONY: imgname
# print the container image name that will be used
# useful for subsequently defining it on the shell
imgname:
	echo IMG=${IMG}

.PHONY: ko-build
# build the container image locally without pushing it to a registry
ko-build:
	CGO_ENABLED=0 ko build --local --bare --tags "$(IMAGE_TAGS)" --platform=$(PLATFORM) .

.PHONY: ko-publish
# build the container image and publish it to the registry named by IMG
ko-publish:
	CGO_ENABLED=0 KO_DOCKER_REPO="$(KO_DOCKER_REPO)" ko build --bare --tags "$(IMAGE_TAGS)" --platform=$(PLATFORM) .

.PHONY: run
# run the ccm locally, really only makes sense on linux anyway
run: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--kubeconfig=${KUBECONFIG}

.PHONY: run-debug
# run the ccm locally, really only makes sense on linux anyway
run-debug: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--kubeconfig=${KUBECONFIG} \
		--linodego-debug

#####################################################################
# E2E Test Setup
#####################################################################

.PHONY: mgmt-and-capl-cluster
mgmt-and-capl-cluster: ko-publish mgmt-cluster
	$(MAKE) -j2 capl-ipv6-cluster capl-cluster

.PHONY: capl-cluster
capl-cluster: generate-capl-cluster-manifests
	MANIFEST_NAME=$(MANIFEST_NAME) CLUSTER_NAME=$(CLUSTER_NAME) KUBECONFIG_PATH=$(KUBECONFIG_PATH) \
		$(MAKE) create-capl-cluster

.PHONY: capl-ipv6-cluster
capl-ipv6-cluster: generate-capl-ipv6-cluster-manifests
	MANIFEST_NAME=$(IPV6_MANIFEST_NAME) CLUSTER_NAME=$(IPV6_CLUSTER_NAME) KUBECONFIG_PATH=$(IPV6_KUBECONFIG_PATH) \
		$(MAKE) create-capl-cluster

.PHONY: generate-capl-cluster-manifests
generate-capl-cluster-manifests:
	# Create the CAPL cluster manifests without any CSI driver stuff
	LINODE_FIREWALL_ENABLED=$(LINODE_FIREWALL_ENABLED) LINODE_OS=$(LINODE_OS) VPC_NAME=$(VPC_NAME) clusterctl generate cluster $(CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) --infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) > $(MANIFEST_NAME).yaml
	IMG=$(IMG) SUBNET_NAME=$(SUBNET_NAME) ./hack/patch-capl-manifest.sh $(MANIFEST_NAME).yaml

.PHONY: generate-capl-ipv6-cluster-manifests
generate-capl-ipv6-cluster-manifests:
	LINODE_FIREWALL_ENABLED=$(LINODE_FIREWALL_ENABLED) LINODE_OS=$(LINODE_OS) VPC_NAME=$(IPV6_CLUSTER_NAME) clusterctl generate cluster $(IPV6_CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) --infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) --flavor kubeadm-dual-stack > $(IPV6_MANIFEST_NAME).yaml
	IMG=$(IMG) ./hack/patch-capl-manifest.sh $(IPV6_MANIFEST_NAME).yaml

.PHONY: create-capl-cluster
create-capl-cluster:
	# Create a CAPL cluster with updated CCM and wait for it to be ready
	kubectl apply -f $(MANIFEST_NAME).yaml
	kubectl wait --for=condition=ControlPlaneReady cluster/$(CLUSTER_NAME) --timeout=600s || (kubectl get cluster -o yaml; kubectl get linodecluster -o yaml; kubectl get linodemachines -o yaml; kubectl logs -n capl-system deployments/capl-controller-manager --tail=50)
	kubectl wait --for=condition=NodeHealthy=true machines -l cluster.x-k8s.io/cluster-name=$(CLUSTER_NAME) --timeout=900s
	clusterctl get kubeconfig $(CLUSTER_NAME) > $(KUBECONFIG_PATH)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl wait --for=condition=Ready nodes --all --timeout=600s
	# Remove all taints from control plane node so that pods scheduled on it by tests can run (without this, some tests fail)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl taint nodes -l node-role.kubernetes.io/control-plane node-role.kubernetes.io/control-plane-

.PHONY: patch-linode-ccm
patch-linode-ccm:
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch -n kube-system daemonset ccm-linode --type='json' -p="[{'op': 'replace', 'path': '/spec/template/spec/containers/0/image', 'value': '${IMG}'}]"
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch -n kube-system daemonset ccm-linode --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/imagePullPolicy", "value": "Always"}]'
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch -n kube-system daemonset ccm-linode --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "LINODE_API_VERSION", "value": "v4beta"}}]'
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl rollout status -n kube-system daemonset/ccm-linode --timeout=600s
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl -n kube-system get daemonset/ccm-linode -o yaml

.PHONY: mgmt-cluster
mgmt-cluster:
	# Create a mgmt cluster
	ctlptl apply -f e2e/setup/ctlptl-config.yaml
	clusterctl init \
		--wait-providers \
		--wait-provider-timeout 600 \
		--core cluster-api:$(CAPI_VERSION) \
		--bootstrap kubeadm:$(CAPI_VERSION) \
		--control-plane kubeadm:$(CAPI_VERSION) \
		--addon helm \
		--infrastructure linode-linode:$(CAPL_VERSION)
	kind get kubeconfig --name=caplccm > $(MGMT_KUBECONFIG_PATH)

.PHONY: cleanup-cluster
cleanup-cluster:
	kubectl delete cluster -A --all --timeout=180s
	kubectl delete linodefirewalls -A --all --timeout=180s
	kubectl delete lvpc -A --all --timeout=180s
	kind delete cluster -n caplccm

.PHONY: e2e-test
e2e-test:
    # Run ipv6 tests first and then the rest
	$(MAKE) e2e-test-ipv6-backends
	CLUSTER_NAME=$(CLUSTER_NAME) \
	MGMT_KUBECONFIG=$(MGMT_KUBECONFIG_PATH) \
	KUBECONFIG=$(KUBECONFIG_PATH) \
	REGION=$(LINODE_REGION) \
	LINODE_TOKEN=$(LINODE_TOKEN) \
	LINODE_URL=$(LINODE_URL) \
	chainsaw test e2e/test --parallel 2 --selector all $(E2E_FLAGS)

.PHONY: e2e-test-ipv6-backends
e2e-test-ipv6-backends:
	CLUSTER_NAME=$(IPV6_CLUSTER_NAME) \
	MGMT_KUBECONFIG=$(MGMT_KUBECONFIG_PATH) \
	KUBECONFIG=$(IPV6_KUBECONFIG_PATH) \
	REGION=$(LINODE_REGION) \
	LINODE_TOKEN=$(LINODE_TOKEN) \
	LINODE_URL=$(LINODE_URL) \
	chainsaw test e2e/test --selector ipv6-backends $(E2E_FLAGS)

.PHONY: e2e-test-bgp
e2e-test-bgp:
	KUBECONFIG=$(KUBECONFIG_PATH) CLUSTER_SUFFIX=$(CLUSTER_NAME) ./e2e/setup/cilium-setup.sh
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl -n kube-system rollout status daemonset/ccm-linode --timeout=300s
	CLUSTER_NAME=$(CLUSTER_NAME) \
		MGMT_KUBECONFIG=$(MGMT_KUBECONFIG_PATH) \
		KUBECONFIG=$(KUBECONFIG_PATH) \
		REGION=$(LINODE_REGION) \
		LINODE_TOKEN=$(LINODE_TOKEN) \
		LINODE_URL=$(LINODE_URL) \
		chainsaw test e2e/bgp-test/lb-cilium-bgp $(E2E_FLAGS)

.PHONY: e2e-test-subnet
e2e-test-subnet: 
	# Generate cluster manifests for second cluster
	SUBNET_NAME=testing CLUSTER_NAME=$(SUBNET_CLUSTER_NAME) MANIFEST_NAME=$(SUBNET_MANIFEST_NAME) VPC_NAME=$(CLUSTER_NAME) \
		VPC_NETWORK_CIDR=172.16.0.0/16 K8S_CLUSTER_CIDR=172.16.64.0/18 make generate-capl-cluster-manifests
	# Create the second cluster
	MANIFEST_NAME=$(SUBNET_MANIFEST_NAME) CLUSTER_NAME=$(SUBNET_CLUSTER_NAME) KUBECONFIG_PATH=$(SUBNET_KUBECONFIG_PATH) \
		make create-capl-cluster
	# Run chainsaw test
	LINODE_TOKEN=$(LINODE_TOKEN) \
		LINODE_URL=$(LINODE_URL) \
		FIRST_CONFIG=$(KUBECONFIG_PATH) \
		SECOND_CONFIG=$(SUBNET_KUBECONFIG_PATH) \
		chainsaw test e2e/subnet-test $(E2E_FLAGS)

.PHONY: helm-lint
helm-lint:
#Verify lint works when region and apiToken are passed, and when it is passed as reference.
	@helm lint deploy/chart --set apiToken="apiToken",region="us-east"
	@helm lint deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east"

.PHONY: helm-template
helm-template:
#Verify template works when region and apiToken are passed, and when it is passed as reference.
	@helm template foo deploy/chart --set apiToken="apiToken",region="us-east" > /dev/null
	@helm template foo deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east" > /dev/null

