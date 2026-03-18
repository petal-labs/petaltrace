# PetalTrace Makefile
# Build and development automation

.PHONY: all build test test-cover coverage lint fmt vet clean install-hooks \
        build-cli install-cli release-build help

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go settings
GO ?= go
GOFLAGS ?=
CGO_ENABLED ?= 0

# Packages
CMD_PKG = github.com/petal-labs/petaltrace/cmd/petaltrace
API_PKG = github.com/petal-labs/petaltrace/api

# Build flags
LDFLAGS = -s -w \
    -X '$(CMD_PKG).Version=$(VERSION)' \
    -X '$(CMD_PKG).Commit=$(COMMIT)' \
    -X '$(CMD_PKG).BuildDate=$(BUILD_DATE)' \
    -X '$(API_PKG).Version=$(VERSION)' \
    -X '$(API_PKG).Commit=$(COMMIT)' \
    -X '$(API_PKG).BuildDate=$(BUILD_DATE)'

# Output directories
BIN_DIR = bin
COVERAGE_DIR = coverage

# Default target
all: lint test build

# Build the CLI binary
build: build-cli

build-cli:
	@echo "Building petaltrace $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/petaltrace ./main.go

# Install the CLI to GOPATH/bin
install-cli:
	@echo "Installing petaltrace $(VERSION)..."
	CGO_ENABLED=$(CGO_ENABLED) $(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./...

# Run all tests
test:
	@echo "Running tests..."
	$(GO) test -race -v ./...

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

# Show coverage summary
coverage: test-cover
	@$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

# Run go vet
vet:
	@echo "Running vet..."
	$(GO) vet ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)
	rm -rf $(COVERAGE_DIR)
	$(GO) clean -cache -testcache

# Install git hooks
install-hooks:
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@cp scripts/pre-commit .git/hooks/pre-commit 2>/dev/null || \
		echo "#!/bin/sh\nmake lint test" > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

# Build release binaries for all platforms
release-build:
	@echo "Building release binaries $(VERSION)..."
	@mkdir -p $(BIN_DIR)

	@echo "  linux/amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/petaltrace-linux-amd64 ./main.go

	@echo "  linux/arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/petaltrace-linux-arm64 ./main.go

	@echo "  darwin/amd64..."
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/petaltrace-darwin-amd64 ./main.go

	@echo "  darwin/arm64..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/petaltrace-darwin-arm64 ./main.go

	@echo "  windows/amd64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/petaltrace-windows-amd64.exe ./main.go

	@echo "Release binaries built in $(BIN_DIR)/"

# Show help
help:
	@echo "PetalTrace Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           Run lint, test, and build (default)"
	@echo "  build         Build the CLI binary"
	@echo "  build-cli     Build the CLI binary"
	@echo "  install-cli   Install the CLI to GOPATH/bin"
	@echo "  test          Run all tests"
	@echo "  test-cover    Run tests with coverage report"
	@echo "  coverage      Show coverage summary"
	@echo "  lint          Run golangci-lint"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  clean         Remove build artifacts"
	@echo "  install-hooks Install git pre-commit hook"
	@echo "  release-build Build binaries for all platforms"
	@echo "  help          Show this help"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=$(VERSION)"
	@echo "  COMMIT=$(COMMIT)"
	@echo "  BUILD_DATE=$(BUILD_DATE)"
