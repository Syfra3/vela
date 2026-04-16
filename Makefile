# Vela Knowledge Graph Builder - Makefile

.PHONY: help build install test test-verbose test-coverage clean fmt lint run dev deps tidy cross release release-check help

# Variables
GOTESTSUM=$(shell go env GOPATH)/bin/gotestsum
BINARY_NAME=vela
BINARY_PATH=./cmd/vela
BUILD_DIR=./bin
INSTALL_PATH=$(HOME)/.local/bin
GO_FILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*")
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-s -w -X main.version=$(VERSION)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

install: build ## Install binary to ~/.local/bin
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/
	@echo "Installed: $(INSTALL_PATH)/$(BINARY_NAME)"

test: ## Run all tests (jest-style summary)
	@if command -v $(GOTESTSUM) >/dev/null 2>&1; then \
		$(GOTESTSUM) --format testdox -- -race -timeout 30s -coverprofile=coverage.out -covermode=atomic ./...; \
	else \
		echo "gotestsum not found. Install with: go install gotest.tools/gotestsum@latest"; \
		exit 1; \
	fi

test-verbose: ## Run tests with verbose output
	@echo "Running tests (verbose)..."
	@go test -v -race -timeout 30s -coverprofile=coverage.out -covermode=atomic ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@$(MAKE) test
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean: ## Remove build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -rf vela-out/
	@echo "Clean complete"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete"

lint: ## Run linter (requires golangci-lint)
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

run: build ## Build and run the binary
	@echo "Running $(BINARY_NAME)..."
	@$(BUILD_DIR)/$(BINARY_NAME)

dev: ## Build in dev mode (no optimization, faster builds)
	@echo "Building $(BINARY_NAME) (dev mode)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	@echo "Dev binary built at $(BUILD_DIR)/$(BINARY_NAME)"

deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "Dependencies ready"

tidy: ## Tidy go.mod and go.sum
	@echo "Tidying modules..."
	@go mod tidy
	@echo "Modules tidied"

cross: ## Cross-compile for multiple platforms
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(BINARY_PATH)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(BINARY_PATH)
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(BINARY_PATH)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)
	@echo "Cross-compilation complete. Binaries in $(BUILD_DIR)/"

release-check: ## Verify everything is ready for release
	@echo "Checking release prerequisites..."
	@if [ -z "$$(git status --porcelain)" ]; then \
		echo "✓ Working directory is clean"; \
	else \
		echo "✗ Working directory has uncommitted changes"; \
		exit 1; \
	fi
	@if git describe --exact-match --tags HEAD >/dev/null 2>&1; then \
		echo "✓ Current commit is tagged"; \
	else \
		echo "✗ Current commit is not tagged. Create a tag first: git tag -a vX.Y.Z -m 'Release vX.Y.Z'"; \
		exit 1; \
	fi
	@echo "✓ All checks passed"

release: release-check test lint ## Create a new release (requires git tag)
	@echo "Triggering release for version $(VERSION)..."
	@echo "Pushing tag to GitHub..."
	git push origin $(VERSION)
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Release triggered! GitHub Actions will:"
	@echo "  1. Build binaries for all platforms"
	@echo "  2. Create GitHub release with artifacts"
	@echo ""
	@echo "Monitor progress: https://github.com/Syfra3/vela/actions"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
