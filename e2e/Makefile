export GO111MODULE=on
IMG ?= linode/linode-cloud-controller-manager:latest

imports: $(GOPATH)/bin/goimports
	goimports -w test

.PHONY: test
test: $(GOPATH)/bin/ginkgo
	@if [ -z "${LINODE_API_TOKEN}" ]; then\
		echo "Skipping Test, LINODE_API_TOKEN is not set";\
	else \
		go list -m; \
		ginkgo -r --v --progress --trace --cover -- --v=3 --image=${IMG}; \
	fi

install-terraform:
	sudo apt-get install wget unzip
	wget https://releases.hashicorp.com/terraform/0.11.13/terraform_0.11.13_linux_amd64.zip
	unzip terraform_0.11.13_linux_amd64.zip
	sudo mv terraform /usr/local/bin/

