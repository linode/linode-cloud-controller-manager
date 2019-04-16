IMG ?= linode/linode-cloud-controller-manager:latest

export GO111MODULE=on

$(GOPATH)/bin/goimports:
	GO111MODULE=off go get golang.org/x/tools/cmd/goimports

$(GOPATH)/bin/ginkgo:
	GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo

vet:
	go vet -composites=false ./...

fmt: vet $(GOPATH)/bin/goimports
	# goimports runs a gofmt
	goimports -w *.go cloud

build: fmt
	go build -o dist/linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

test: $(GOPATH)/bin/ginkgo
	ginkgo -r --v --progress --trace --cover --skipPackage=test -- --v=3

docker-build: test build
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}

run: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--cloud-provider=linode \
		--kubeconfig=${KUBECONFIG}

run-debug: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--cloud-provider=linode \
		--kubeconfig=${KUBECONFIG} \
		--linodego-debug

all: build

