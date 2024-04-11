ROOT_DIR="$(shell git rev-parse --show-toplevel)"

IMG ?= linode/linode-cloud-controller-manager:latest
K8S_VERSION?="v1.28.7"

REUSED_KUBECONFIG := test/ccm-linode-for-reuse.conf
MANAGEMENT_KUBECONFIG := test/kind-management.conf

ifneq ("$(wildcard $(REUSED_KUBECONFIG))","")
    CONTROL_PLANE_IP := $(shell grep -E -o "([0-9]{1,3}[\.]){3}[0-9]{1,3}" $(REUSED_KUBECONFIG))
endif

.PHONY: test reuse-and-test clean

test-cluster: ctlptl kind clean-cluster
	./test/scripts/create_management_cluster.sh $(K8S_VERSION)

clean-cluster: ctlptl
	ctlptl delete cluster kind-management --ignore-not-found
	rm -rf $(MANAGEMENT_KUBECONFIG)

reuse-and-test: SUITE_ARGS='--reuse'

test reuse-and-test: clusterctl ginkgo check-token test-cluster
	go list -m; \
	MANAGEMENT_KUBECONFIG=$(ROOT_DIR)/e2e/$(MANAGEMENT_KUBECONFIG) ginkgo -r --vv --trace $(TEST_ARGS) -- --image=${IMG} --k8s_version=${K8S_VERSION} $(SUITE_ARGS)

	$(MAKE) clean-cluster

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh ccm-linode-for-reuse

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi

control-plane-ssh: $(REUSED_KUBECONFIG)
	ssh root@$(CONTROL_PLANE_IP)

#####################################################################
# OS / ARCH
#####################################################################
OS=$(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(shell uname -m)
ARCH_SHORT=$(ARCH)
ifeq ($(ARCH_SHORT),x86_64)
ARCH_SHORT := amd64
else ifeq ($(ARCH_SHORT),aarch64)
ARCH_SHORT := arm64
endif

## --------------------------------------
## Build Dependencies
## --------------------------------------

##@ Build Dependencies:

## Location to install dependencies to

# Use CACHE_BIN for tools that cannot use devbox and LOCALBIN for tools that can use either method
CACHE_BIN ?= $(CURDIR)/../bin
LOCALBIN ?= $(CACHE_BIN)

DEVBOX_BIN ?= $(DEVBOX_PACKAGES_DIR)/bin

# if the $DEVBOX_PACKAGES_DIR env variable exists that means we are within a devbox shell and can safely
# use devbox's bin for our tools
ifdef DEVBOX_PACKAGES_DIR
	LOCALBIN = $(DEVBOX_BIN)
endif

export PATH := $(CACHE_BIN):$(PATH)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

##@ Tooling Binaries:
GINKGO         ?= $(LOCALBIN)/ginkgo
CTLPTL         ?= $(LOCALBIN)/ctlptl
CLUSTERCTL     ?= $(LOCALBIN)/clusterctl
KIND           ?= $(LOCALBIN)/kind

## Tool Versions
GINKGO_VERSION           ?= v2.17.0
CTLPTL_VERSION           ?= v0.8.28
CLUSTERCTL_VERSION       ?= v1.6.3
KIND_VERSION             ?= v0.22.0

.PHONY: tools
tools: ginkgo ctlptl clusterctl

.PHONY: ginkgo
ginkgo: $(GINKGO)
$(GINKGO): $(LOCALBIN) ## Download ginkgo locally if necessary.
	GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

.PHONY: ctlptl
ctlptl: $(CTLPTL) ## Download ctlptl locally if necessary.
$(CTLPTL): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/tilt-dev/ctlptl/cmd/ctlptl@$(CTLPTL_VERSION)

.PHONY: clusterctl
clusterctl: $(CLUSTERCTL) ## Download clusterctl locally if necessary.
$(CLUSTERCTL): $(LOCALBIN)
	curl -fsSL https://github.com/kubernetes-sigs/cluster-api/releases/download/$(CLUSTERCTL_VERSION)/clusterctl-$(OS)-$(ARCH_SHORT) -o $(CLUSTERCTL)
	chmod +x $(CLUSTERCTL)

.PHONY: kind
kind: $(KIND) ## Download kustomize locally if necessary.
$(KIND): $(LOCALBIN)
	curl -fsSL https://github.com/kubernetes-sigs/kind/releases/download/$(KIND_VERSION)/kind-$(OS)-$(ARCH_SHORT) -o $(KIND)
	chmod +x $(KIND)
