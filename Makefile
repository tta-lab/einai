.PHONY: help build clean test install reinstall setup run fmt tidy qlty all ci check-clean install-hooks

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION := $(shell go version | awk '{print $$3}')

LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.goVersion=$(GO_VERSION)

help:
	@echo "Available commands:"
	@echo "  make build         - Build the ei binary"
	@echo "  make install       - Install ei to GOPATH/bin"
	@echo "  make reinstall     - Install and restart daemon"
	@echo "  make setup         - First-time setup (install + daemon)"
	@echo "  make run           - Run ei (usage: make run ARGS='agent list')"
	@echo "  make clean         - Remove built binaries"
	@echo "  make test          - Run tests"
	@echo "  make fmt           - Format code with gofmt"
	@echo "  make tidy          - Tidy go modules"
	@echo "  make qlty          - Run qlty check (lint + security scan)"
	@echo "  make all           - Format, tidy, qlty, and build"
	@echo "  make ci            - Run all CI checks (qlty, test, build)"
	@echo "  make check-clean   - Check if working directory is clean"
	@echo "  make install-hooks  - Install qlty git hooks"

build:
	@echo "Building ei..."
	@go build -ldflags "$(LDFLAGS)" -o ei .
	@echo "✓ Build complete: ./ei"

install:
	@echo "Installing ei..."
	@go build -ldflags "$(LDFLAGS)" -o $(shell go env GOPATH)/bin/ei .
	@echo "✓ Installed to $(shell go env GOPATH)/bin/ei"

reinstall: install
	@for label in $$(launchctl list 2>/dev/null | awk '/io\.tta\.einai\.daemon/ {print $$3}'); do \
		echo "Restarting $$label..."; \
		launchctl kickstart -k "gui/$$(id -u)/$$label"; \
		echo "✓ $$label restarted"; \
	done

setup: install
	@echo "Setting up einai..."
	@$(shell go env GOPATH)/bin/ei daemon install
	@echo "✓ Setup complete"

run: build
	@./ei $(ARGS)

clean:
	@echo "Cleaning build artifacts..."
	@rm -f ei
	@echo "✓ Cleaned"

test:
	@echo "Running tests..."
	@go test -v ./...

tidy:
	@echo "Tidying go modules..."
	@go mod tidy
	@echo "✓ go mod tidy complete"

fmt:
	@echo "Formatting code..."
	@gofmt -w -s .
	@echo "✓ Code formatted"

qlty:
	@echo "Running qlty check..."
	@qlty check --all --no-progress
	@echo "✓ Qlty check complete"

all: fmt tidy qlty build
	@echo "✓ All checks passed and binary built"

ci: qlty test build
	@echo "✓ CI checks complete"

check-clean:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "❌ Working directory is not clean"; \
		git status --short; \
		exit 1; \
	else \
		echo "✓ Working directory is clean"; \
	fi

install-hooks:
	@qlty githooks install
	@echo "✓ Qlty hooks installed"
