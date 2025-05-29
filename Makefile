IMG                     ?= linode/linode-cloud-controller-manager:canary
RELEASE_DIR             ?= release
PLATFORM                ?= linux/amd64

# Use CACHE_BIN for tools that cannot use devbox and LOCALBIN for tools that can use either method
CACHE_BIN               ?= $(CURDIR)/bin
LOCALBIN                ?= $(CACHE_BIN)

DEVBOX_BIN              ?= $(DEVBOX_PACKAGES_DIR)/bin
HELM                    ?= $(LOCALBIN)/helm
HELM_VERSION            ?= v3.16.3

#####################################################################
# Dev Setup
#####################################################################
CLUSTER_NAME            ?= ccm-$(shell git rev-parse --short HEAD)
SUBNET_CLUSTER_NAME     ?= subnet-testing-$(shell git rev-parse --short HEAD)
VPC_NAME                ?= $(CLUSTER_NAME)
MANIFEST_NAME           ?= capl-cluster-manifests
SUBNET_MANIFEST_NAME    ?= subnet-testing-manifests
K8S_VERSION             ?= "v1.31.2"
CAPI_VERSION            ?= "v1.8.5"
CAAPH_VERSION           ?= "v0.2.1"
CAPL_VERSION            ?= "v0.8.5"
CONTROLPLANE_NODES      ?= 1
WORKER_NODES            ?= 1
LINODE_FIREWALL_ENABLED ?= true
LINODE_REGION           ?= us-lax
LINODE_OS               ?= linode/ubuntu22.04
LINODE_URL              ?= https://api.linode.com
KUBECONFIG_PATH         ?= $(CURDIR)/test-cluster-kubeconfig.yaml
SUBNET_KUBECONFIG_PATH	?= $(CURDIR)/subnet-testing-kubeconfig.yaml
MGMT_KUBECONFIG_PATH    ?= $(CURDIR)/mgmt-cluster-kubeconfig.yaml
GOLANGCI_LINT_VERSION   ?= v2.1.5

# if the $DEVBOX_PACKAGES_DIR env variable exists that means we are within a devbox shell and can safely
# use devbox's bin for our tools
ifdef DEVBOX_PACKAGES_DIR
	LOCALBIN = $(DEVBOX_BIN)
endif

export PATH := $(CACHE_BIN):$(PATH)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

export GO111MODULE=on

.PHONY: all
all: build

.PHONY: clean
clean:
	@go clean .
	@rm -rf ./.tmp
	@rm -rf dist/*
	@rm -rf $(RELEASE_DIR)
	@rm -rf $(LOCALBIN)

.PHONY: codegen
codegen:
	go generate ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: lint
lint:
	docker run --rm -w /workdir -v $(PWD):/workdir golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint run -c .golangci.yml --fix

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
	go test -v -coverpkg=./sentry,./cloud/linode/client,./cloud/linode/firewall,./cloud/linode -coverprofile ./coverage.out -cover ./sentry/... ./cloud/... $(TEST_ARGS)

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
# print the Docker image name that will be used
# useful for subsequently defining it on the shell
imgname:
	echo IMG=${IMG}

.PHONY: docker-build
# we cross compile the binary for linux, then build a container
docker-build: build-linux
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --tag ${IMG} .

.PHONY: docker-push
# must run the docker build before pushing the image
docker-push:
	docker push ${IMG}

.PHONY: docker-setup
docker-setup: docker-build docker-push

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
mgmt-and-capl-cluster: docker-setup mgmt-cluster capl-cluster

.PHONY: capl-cluster
capl-cluster: generate-capl-cluster-manifests create-capl-cluster patch-linode-ccm

.PHONY: generate-capl-cluster-manifests
generate-capl-cluster-manifests:
	# Create the CAPL cluster manifests without any CSI driver stuff
	LINODE_FIREWALL_ENABLED=$(LINODE_FIREWALL_ENABLED) LINODE_OS=$(LINODE_OS) VPC_NAME=$(VPC_NAME) clusterctl generate cluster $(CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) --infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) > $(MANIFEST_NAME).yaml
	yq -i e 'select(.kind == "LinodeVPC").spec.subnets = [{"ipv4": "10.0.0.0/8", "label": "default"}, {"ipv4": "172.16.0.0/16", "label": "testing"}]' $(MANIFEST_NAME).yaml

.PHONY: create-capl-cluster
create-capl-cluster:
	# Create a CAPL cluster with updated CCM and wait for it to be ready
	kubectl apply -f $(MANIFEST_NAME).yaml
	kubectl wait --for=condition=ControlPlaneReady cluster/$(CLUSTER_NAME) --timeout=600s || (kubectl get cluster -o yaml; kubectl get linodecluster -o yaml; kubectl get linodemachines -o yaml)
	kubectl wait --for=condition=NodeHealthy=true machines -l cluster.x-k8s.io/cluster-name=$(CLUSTER_NAME) --timeout=900s
	clusterctl get kubeconfig $(CLUSTER_NAME) > $(KUBECONFIG_PATH)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl wait --for=condition=Ready nodes --all --timeout=600s
	# Remove all taints from control plane node so that pods scheduled on it by tests can run (without this, some tests fail)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl taint nodes -l node-role.kubernetes.io/control-plane node-role.kubernetes.io/control-plane-

.PHONY: patch-linode-ccm
patch-linode-ccm:
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch -n kube-system daemonset ccm-linode --type='json' -p="[{'op': 'replace', 'path': '/spec/template/spec/containers/0/image', 'value': '${IMG}'}]"
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
		--addon helm:$(CAAPH_VERSION) \
		--infrastructure linode-linode:$(CAPL_VERSION)
	kind get kubeconfig --name=caplccm > $(MGMT_KUBECONFIG_PATH)

.PHONY: cleanup-cluster
cleanup-cluster:
	kubectl delete cluster -A --all --timeout=180s
	kubectl delete linodefirewalls -A --all --timeout=60s
	kubectl delete lvpc -A --all --timeout=60s
	kind delete cluster -n caplccm

.PHONY: e2e-test
e2e-test:
	CLUSTER_NAME=$(CLUSTER_NAME) \
	MGMT_KUBECONFIG=$(MGMT_KUBECONFIG_PATH) \
	KUBECONFIG=$(KUBECONFIG_PATH) \
	REGION=$(LINODE_REGION) \
	LINODE_TOKEN=$(LINODE_TOKEN) \
	LINODE_URL=$(LINODE_URL) \
	chainsaw test e2e/test --parallel 2 $(E2E_FLAGS)

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
	# Add subnetNames to HelmChartProxy	
	yq e 'select(.kind == "HelmChartProxy" and .spec.chartName == "ccm-linode").spec.valuesTemplate' $(SUBNET_MANIFEST_NAME).yaml > tmp.yaml
	yq -i e '.routeController  += {"subnetNames": "testing"}' tmp.yaml
	yq -i e '.routeController.vpcNames = "{{.InfraCluster.spec.vpcRef.name}}"' tmp.yaml
	yq -i e 'select(.kind == "HelmChartProxy" and .spec.chartName == "ccm-linode").spec.valuesTemplate = load_str("tmp.yaml")' $(SUBNET_MANIFEST_NAME).yaml
	rm tmp.yaml
	# Create the second cluster
	MANIFEST_NAME=$(SUBNET_MANIFEST_NAME) CLUSTER_NAME=$(SUBNET_CLUSTER_NAME) KUBECONFIG_PATH=$(SUBNET_KUBECONFIG_PATH) \
		make create-capl-cluster
	KUBECONFIG_PATH=$(SUBNET_KUBECONFIG_PATH) make patch-linode-ccm
	# Run chainsaw test
	LINODE_TOKEN=$(LINODE_TOKEN) \
		LINODE_URL=$(LINODE_URL) \
		FIRST_CONFIG=$(KUBECONFIG_PATH) \
		SECOND_CONFIG=$(SUBNET_KUBECONFIG_PATH) \
		chainsaw test e2e/subnet-test $(E2E_FLAGS)

#####################################################################
# OS / ARCH
#####################################################################

# Set the host's OS. Only linux and darwin supported for now
HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ifeq ($(filter darwin linux,$(HOSTOS)),)
$(error build only supported on linux and darwin host currently)
endif
ARCH=$(shell uname -m)
ARCH_SHORT=$(ARCH)
ifeq ($(ARCH_SHORT),x86_64)
ARCH_SHORT := amd64
else ifeq ($(ARCH_SHORT),aarch64)
ARCH_SHORT := arm64
endif

.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary
$(HELM): $(LOCALBIN)
	@curl -fsSL https://get.helm.sh/helm-$(HELM_VERSION)-$(HOSTOS)-$(ARCH_SHORT).tar.gz | tar -xz
	@mv $(HOSTOS)-$(ARCH_SHORT)/helm $(HELM)
	@rm -rf helm.tgz $(HOSTOS)-$(ARCH_SHORT)

.PHONY: helm-lint
helm-lint: helm
#Verify lint works when region and apiToken are passed, and when it is passed as reference.
	@$(HELM) lint deploy/chart --set apiToken="apiToken",region="us-east"
	@$(HELM) lint deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east"

.PHONY: helm-template
helm-template: helm
#Verify template works when region and apiToken are passed, and when it is passed as reference.
	@$(HELM) template foo deploy/chart --set apiToken="apiToken",region="us-east" > /dev/null
	@$(HELM) template foo deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east" > /dev/null
