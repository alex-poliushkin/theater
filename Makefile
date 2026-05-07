SHELL := /bin/bash

BUILD_DIR := build
BINARY := $(BUILD_DIR)/theater
LSP_BINARY := $(BUILD_DIR)/thtr-lsp
GOLANGCI_IMAGE := golangci/golangci-lint:v2.5.0

.PHONY: build build-dir build-lsp clean fmt lint test

build: $(BINARY)

build-lsp: $(LSP_BINARY)

clean:
	rm -rf $(BUILD_DIR)

fmt:
	gofmt -w .

lint:
	docker run --rm -v "$(PWD)":/app:ro -w /app $(GOLANGCI_IMAGE) golangci-lint run ./...

test:
	go test ./...

$(BINARY): | build-dir
	go build -o $(BINARY) ./cmd/theater

$(LSP_BINARY): | build-dir
	go build -o $(LSP_BINARY) ./cmd/thtr-lsp

build-dir:
	mkdir -p $(BUILD_DIR)
