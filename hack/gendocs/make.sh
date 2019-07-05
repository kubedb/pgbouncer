#!/usr/bin/env bash

pushd $GOPATH/src/github.com/kubedb/pgbouncer/hack/gendocs
go run main.go
popd
