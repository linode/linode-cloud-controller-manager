#!/usr/bin/env bash

pushd $GOPATH/src/github.com/pharmer/ccm-linode/hack/gendocs
go run main.go
popd
