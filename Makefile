IMG ?= linode/linode-cloud-controller-manager:latest

all: build

build: fmt
	go build -o dist/linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

run: build
	dist/linode-cloud-controller-manager --logtostderr=true --stderrthreshold=INFO

fmt:
	go vet -composites=false ./...
	goimports -w *.go cloud
	gofmt -s -w *.go cloud

test:
	ginkgo -r --v --progress --trace -- --v=3

docker-build:
	docker build . -t ${IMG}

docker-push:
	docker push ${IMG}
