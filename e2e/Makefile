export GO111MODULE=on
IMG ?= linode/linode-cloud-controller-manager:latest

REUSED_KUBECONFIG := test/ccm-linode-for-reuse.conf

ifneq ("$(wildcard $(REUSED_KUBECONFIG))","")
    CONTROL_PLANE_IP := $(shell grep -E -o "([0-9]{1,3}[\.]){3}[0-9]{1,3}" $(REUSED_KUBECONFIG))
endif

imports: $(GOPATH)/bin/goimports
	goimports -w test

.PHONY: test reuse-and-test clean

reuse-and-test: SUITE_ARGS='--reuse'

test reuse-and-test: $(GOPATH)/bin/ginkgo check-token
	go list -m; \
	ginkgo -r --v --progress --trace --cover $(TEST_ARGS) -- --v=3 --image=${IMG} $(SUITE_ARGS); \

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

install-terraform:
	sudo apt-get install wget unzip
	wget https://releases.hashicorp.com/terraform/0.11.13/terraform_0.11.13_linux_amd64.zip
	unzip terraform_0.11.13_linux_amd64.zip
	sudo mv terraform /usr/local/bin/

control-plane-ssh: $(REUSED_KUBECONFIG)
	ssh core@$(CONTROL_PLANE_IP)
