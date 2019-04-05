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