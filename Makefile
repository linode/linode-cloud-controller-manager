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
K8S_VERSION             ?= "v1.31.2"
CAPI_VERSION            ?= "v1.8.5"
CAAPH_VERSION           ?= "v0.2.1"
CAPL_VERSION            ?= "v0.7.1"
CONTROLPLANE_NODES      ?= 1
WORKER_NODES            ?= 1
LINODE_FIREWALL_ENABLED ?= true
LINODE_REGION           ?= us-lax
LINODE_OS               ?= linode/ubuntu22.04
KUBECONFIG_PATH         ?= $(CURDIR)/test-cluster-kubeconfig.yaml
MGMT_KUBECONFIG_PATH    ?= $(CURDIR)/mgmt-cluster-kubeconfig.yaml

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
	docker run --rm -v "$(PWD):/var/work:ro" -w /var/work \
		golangci/golangci-lint:latest golangci-lint run -c .golangci.yml

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
	go test -v -cover -coverprofile ./coverage.out ./cloud/... $(TEST_ARGS)

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
	LINODE_FIREWALL_ENABLED=$(LINODE_FIREWALL_ENABLED) LINODE_OS=$(LINODE_OS) clusterctl generate cluster $(CLUSTER_NAME) \
		--kubernetes-version $(K8S_VERSION) --infrastructure linode-linode:$(CAPL_VERSION) \
		--control-plane-machine-count $(CONTROLPLANE_NODES) --worker-machine-count $(WORKER_NODES) > capl-cluster-manifests.yaml

.PHONY: create-capl-cluster
create-capl-cluster:
	# Create a CAPL cluster with updated CCM and wait for it to be ready
	kubectl apply -f capl-cluster-manifests.yaml
	kubectl wait --for=condition=ControlPlaneReady cluster/$(CLUSTER_NAME) --timeout=600s || (kubectl get cluster -o yaml; kubectl get linodecluster -o yaml; kubectl get linodemachines -o yaml)
	kubectl wait --for=condition=NodeHealthy=true machines -l cluster.x-k8s.io/cluster-name=$(CLUSTER_NAME) --timeout=900s
	clusterctl get kubeconfig $(CLUSTER_NAME) > $(KUBECONFIG_PATH)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl wait --for=condition=Ready nodes --all --timeout=600s
	# Remove all taints from control plane node so that pods scheduled on it by tests can run (without this, some tests fail)
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl taint nodes -l node-role.kubernetes.io/control-plane node-role.kubernetes.io/control-plane-

.PHONY: patch-linode-ccm
patch-linode-ccm:
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch -n kube-system daemonset ccm-linode --type='json' -p="[{'op': 'replace', 'path': '/spec/template/spec/containers/0/image', 'value': '${IMG}'}]"
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
	chainsaw test e2e/test --parallel 2

.PHONY: e2e-test-bgp
e2e-test-bgp:
	# Debugging: Print the nodes in the cluster
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl get nodes -o wide
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl get nodes --no-headers |\
	 grep -v control-plane | awk '{print $$1}'

	# Add bgp peering label to non control plane nodes
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl get nodes --no-headers | grep -v control-plane |\
	 awk '{print $$1}' | xargs -I {} env KUBECONFIG=$(KUBECONFIG_PATH) kubectl label nodes {} cilium-bgp-peering=true --overwrite

	# First patch: Add the necessary RBAC permissions
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch clusterrole ccm-linode-clusterrole \
		--type='json' \
		-p='[\
			{\
				"op": "add",\
				"path": "/rules/-",\
				"value": {\
					"apiGroups": ["cilium.io"],\
					"resources": ["ciliumloadbalancerippools", "ciliumbgppeeringpolicies"],\
					"verbs": ["get", "list", "watch", "create", "update", "patch", "delete"]\
				}\
			}\
		]'
	
	# Patch: Append new args to the existing ones
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl patch daemonset ccm-linode -n kube-system \
		--type='json' \
		-p='[\
			{\
				"op": "add",\
				"path": "/spec/template/spec/containers/0/args/-",\
				"value": "--bgp-node-selector=cilium-bgp-peering=true"\
			},\
			{\
				"op": "add",\
				"path": "/spec/template/spec/containers/0/args/-",\
				"value": "--load-balancer-type=cilium-bgp"\
			},\
			{\
				"op": "add",\
				"path": "/spec/template/spec/containers/0/args/-",\
				"value": "--ip-holder-suffix=ccm-8039dcf"\
			}\
		]'
		
	# Wait for rollout
	KUBECONFIG=$(KUBECONFIG_PATH) kubectl -n kube-system rollout status daemonset/ccm-linode --timeout=300s
	
	# Run the tests
	CLUSTER_NAME=$(CLUSTER_NAME) \
		MGMT_KUBECONFIG=$(MGMT_KUBECONFIG_PATH) \
		KUBECONFIG=$(KUBECONFIG_PATH) \
		REGION=$(LINODE_REGION) \
		LINODE_TOKEN=$(LINODE_TOKEN) \
		chainsaw test e2e/lb-cilium-bgp

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
