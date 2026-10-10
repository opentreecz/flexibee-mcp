# Makefile for github.com/opentreecz/flexibee-mcp
# https://github.com/opentreecz/flexibee-mcp

BINARY     := flexibee-mcp
MODULE     := github.com/opentreecz/flexibee-mcp
CMD        := ./cmd/flexibee-mcp/

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS    := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -s -w"

DIST_DIR   := dist
DIST_LINUX := $(DIST_DIR)/$(BINARY)-linux-amd64
DIST_WIN   := $(DIST_DIR)/$(BINARY)-windows-amd64.exe

.PHONY: all build build-linux build-windows dist test vet fmt fmt-check lint tidy clean ci help

all: build

## build: compile the binary for the current platform
build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

## build-linux: cross-compile a static Linux amd64 binary
build-linux:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_LINUX) $(CMD)

## build-windows: cross-compile a static Windows amd64 binary
build-windows:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_WIN) $(CMD)

## dist: build release binaries and generate sha256 checksums
dist: build-linux build-windows
	cd $(DIST_DIR) && sha256sum $(BINARY)-linux-amd64 $(BINARY)-windows-amd64.exe > checksums.txt

## test: run all tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go source files
fmt:
	gofmt -w .

## fmt-check: verify formatting (non-zero exit if files need formatting; used in CI)
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need formatting (run 'make fmt'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## lint: static analysis via go vet
lint:
	go vet ./...

## tidy: tidy go.mod and go.sum
tidy:
	go mod tidy

## clean: remove build artefacts
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DIST_DIR)

## ci: run checks suitable for a CI pipeline (fmt-check, vet, test)
ci: fmt-check vet test

## help: list available targets
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
