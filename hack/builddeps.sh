#!/bin/bash

go get -u golang.org/x/tools/cmd/goimports
go get github.com/onsi/ginkgo/ginkgo
go install github.com/onsi/ginkgo/ginkgo
go get -u github.com/jteeuwen/go-bindata/...
