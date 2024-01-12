IMG ?= linode/linode-cloud-controller-manager:canary
RELEASE_DIR ?= release
GOLANGCI_LINT_IMG := golangci/golangci-lint:v1.55-alpine
PLATFORM ?= linux/amd64

export GO111MODULE=on

.PHONY: all
all: build

.PHONY: clean
clean:
	@go clean .
	@rm -rf ./.tmp
	@rm -rf dist/*
	@rm -rf $(RELEASE_DIR)

.PHONY: codegen
codegen:
	go generate ./...

.PHONY: vet
vet: fmt
	go vet ./...

.PHONY: lint
lint:
	docker run --rm -v "$(shell pwd):/var/work:ro" -w /var/work \
		golangci/golangci-lint:v1.55.2 golangci-lint run -v --timeout=5m
	docker run --rm -v "$(shell pwd):/var/work:ro" -w /var/work/e2e \
		golangci/golangci-lint:v1.55.2 golangci-lint run -v --timeout=5m

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: test
# we say code is not worth testing unless it's formatted
test: fmt codegen
	go test -v -cover ./cloud/... $(TEST_ARGS)

.PHONY: build-linux
build-linux: codegen
	echo "compiling linode-cloud-controller-manager for linux/amd64" && \
		GOOS=linux GOARCH=amd64 \
		CGO_ENABLED=0 \
		go build -ldflags '-extldflags "-static"' -o dist/linode-cloud-controller-manager-linux-amd64 .

.PHONY: build
build: codegen
	echo "compiling linode-cloud-controller-manager" && \
		CGO_ENABLED=0 \
		go build -o dist/linode-cloud-controller-manager .

.PHONY: release
release:
	mkdir -p $(RELEASE_DIR)
	sed -e 's/appVersion: "latest"/appVersion: "$(IMAGE_VERSION)"/g' ./deploy/chart/Chart.yaml
	tar -czvf ./$(RELEASE_DIR)/helm-chart-$(IMAGE_VERSION).tgz -C ./deploy/chart .

.PHONY: imgname
# print the Docker image name that will be used
# useful for subsequently defining it on the shell
imgname:
	echo IMG=${IMG}

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --platform=$(PLATFORM) --tag ${IMG} .

.PHONY: docker-push
# must run the docker build before pushing the image
docker-push:
	echo "[reminder] Did you run `make docker-build`?"
	docker push ${IMG}

.PHONY: run
# run the ccm locally, really only makes sense on linux anyway
run: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--kubeconfig=${KUBECONFIG}

.PHONY: run-debug
# run the ccm locally, really only makes sense on linux anyway
run-debug: build
	dist/linode-cloud-controller-manager \
		--logtostderr=true \
		--stderrthreshold=INFO \
		--kubeconfig=${KUBECONFIG} \
		--linodego-debug
# Set the host's OS. Only linux and darwin supported for now
HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ifeq ($(filter darwin linux,$(HOSTOS)),)
$(error build only supported on linux and darwin host currently)
endif

HELM_VERSION ?= v3.9.1
TOOLS_HOST_DIR ?= .tmp/tools
HELM := $(TOOLS_HOST_DIR)/helm-$(HELM_VERSION)

.PHONY: $(HELM)
$(HELM):
	@echo installing helm $(HELM_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp-helm
	@curl -fsSL https://get.helm.sh/helm-$(HELM_VERSION)-$(HOSTOS)-amd64.tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp-helm
	@mv $(TOOLS_HOST_DIR)/tmp-helm/$(HOSTOS)-amd64/helm $(HELM)
	@rm -fr $(TOOLS_HOST_DIR)/tmp-helm
	@echo installing helm $(HELM_VERSION)

.PHONY: helm-lint
helm-lint: $(HELM)
#Verify lint works when region and apiToken are passed, and when it is passed as reference.
	@$(HELM) lint deploy/chart --set apiToken="apiToken",region="us-east"
	@$(HELM) lint deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east"

.PHONY: helm-template
helm-template: $(HELM)
#Verify template works when region and apiToken are passed, and when it is passed as reference.
	@$(HELM) template foo deploy/chart --set apiToken="apiToken",region="us-east" > /dev/null
	@$(HELM) template foo deploy/chart --set secretRef.apiTokenRef="apiToken",secretRef.name="api",secretRef.regionRef="us-east" > /dev/null
