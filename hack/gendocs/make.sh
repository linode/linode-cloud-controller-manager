#!/usr/bin/env bash

pushd $GOPATH/src/github.com/linode/linode-cloud-controller-manager/hack/gendocs
go run main.go
popd
