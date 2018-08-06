#!/usr/bin/env bash

set -e

go install ./vendor/github.com/mna/pigeon
go install ./vendor/golang.org/x/tools/cmd/goimports
go install ./vendor/golang.org/x/lint/golint
go install ./vendor/golang.org/x/perf/cmd/benchstat
