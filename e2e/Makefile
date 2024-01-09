IMG ?= linode/linode-cloud-controller-manager:latest
GINKGO_PATH := $(shell go env GOPATH)/bin/ginkgo

REUSED_KUBECONFIG := test/ccm-linode-for-reuse.conf

ifneq ("$(wildcard $(REUSED_KUBECONFIG))","")
    CONTROL_PLANE_IP := $(shell grep -E -o "([0-9]{1,3}[\.]){3}[0-9]{1,3}" $(REUSED_KUBECONFIG))
endif

.PHONY: test reuse-and-test clean

${GINKGO_PATH}:
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.13.2

reuse-and-test: SUITE_ARGS='--reuse'

test reuse-and-test: ${GINKGO_PATH} check-token
	go list -m; \
	ginkgo -r --vv --trace $(TEST_ARGS) -- --image=${IMG} $(SUITE_ARGS)

clean: check-token
	cd test; \
	./scripts/delete_cluster.sh ccm-linode-for-reuse; \
	rm terraform.tfstate; \
    rm -rf terraform.tfstate.d

check-token:
	@if test "$(LINODE_API_TOKEN)" = "" ; then \
	  echo "LINODE_API_TOKEN must be set"; \
	  exit 1; \
	fi

control-plane-ssh: $(REUSED_KUBECONFIG)
	ssh root@$(CONTROL_PLANE_IP)
