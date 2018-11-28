IMG ?= linode/linode-cloud-controller-manager:latest

all: build

build: fmt
	go build -o dist/linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

run: build
	dist/linode-cloud-controller-manager --logtostderr=true --stderrthreshold=INFO

$(GOPATH)/bin/goimports:
	go get golang.org/x/tools/cmd/goimports

vet:
	go vet -composites=false ./...

imports: $(GOPATH)/bin/goimports
	goimports -w *.go cloud

fmt: vet imports
	gofmt -s -w *.go cloud

$(GOPATH)/bin/ginkgo:
	go get -u github.com/onsi/ginkgo/ginkgo

test: $(GOPATH)/bin/ginkgo
	ginkgo -r --v --progress --trace -- --v=3

docker-build:
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}
