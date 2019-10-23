#!/usr/bin/env bash

pushd $GOPATH/src/kubedb.dev/pgbouncer/hack/gendocs
go run main.go
popd
