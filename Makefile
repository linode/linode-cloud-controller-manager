IMG ?= linode/linode-cloud-controller-manager:latest

export GO111MODULE=on

all: build

build: fmt
	go build -o dist/linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

run: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--cloud-provider=linode \
		--kubeconfig=${KUBECONFIG}

$(GOPATH)/bin/goimports:
	GO111MODULE=off go get golang.org/x/tools/cmd/goimports

vet:
	go vet -composites=false ./...

imports: $(GOPATH)/bin/goimports
	goimports -w *.go cloud

fmt: vet imports
	gofmt -s -w *.go cloud

$(GOPATH)/bin/ginkgo:
	GO111MODULE=off go get -u github.com/onsi/ginkgo/ginkgo

test: $(GOPATH)/bin/ginkgo
	ginkgo -r --v --progress --trace --cover -- --v=3

docker-build:
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}
