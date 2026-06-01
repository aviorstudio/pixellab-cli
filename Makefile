.PHONY: build install test

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	mkdir -p bin
	go build -ldflags '$(LDFLAGS)' -o bin/pxlb .

install:
	mkdir -p bin
	mkdir -p "$$(go env GOPATH)/bin"
	go build -ldflags '$(LDFLAGS)' -o bin/pxlb .
	cp bin/pxlb "$$(go env GOPATH)/bin/pxlb"

test:
	go test ./...
