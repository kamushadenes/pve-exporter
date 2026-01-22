.PHONY: all build test lint clean help

BINARY_NAME=pve-exporter

all: lint test build

build:
	go build -o $(BINARY_NAME) -v

test:
	go test -v -race ./...

lint:
	golangci-lint run

clean:
	go clean
	rm -f $(BINARY_NAME)

help:
	@echo "Makefile commands:"
	@echo "  make build    - Build the binary"
	@echo "  make test     - Run tests"
	@echo "  make lint     - Run linter"
	@echo "  make clean    - Clean build artifacts"
	@echo "  make all      - Run lint, test, and build"
