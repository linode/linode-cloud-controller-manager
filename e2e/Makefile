export GO111MODULE=on

imports: $(GOPATH)/bin/goimports
	goimports -w *.go cloud

gingko_test: $(GOPATH)/bin/ginkgo
	@if [ -z "${LINODE_API_TOKEN}" ]; then\
		echo "Skipping Test, LINODE_API_TOKEN is not set";\
	else \
		go list -m; \
		ginkgo -r --v --progress --trace --cover -- --v=3; \
	fi

install-terraform:
	sudo apt-get install wget unzip
	wget https://releases.hashicorp.com/terraform/0.11.13/terraform_0.11.13_linux_amd64.zip
	unzip terraform_0.11.13_linux_amd64.zip
	sudo mv terraform /usr/local/bin/