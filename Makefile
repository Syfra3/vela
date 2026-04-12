# Vela Knowledge Graph Builder - Makefile

.PHONY: help build install test test-verbose test-coverage clean fmt lint run dev deps

# Variables
BINARY_NAME=vela
BINARY_PATH=./cmd/vela
BUILD_DIR=./bin
INSTALL_PATH=$(HOME)/.local/bin
GO_FILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

install: build ## Install binary to ~/.local/bin
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/
	@echo "Installed: $(INSTALL_PATH)/$(BINARY_NAME)"

test: ## Run all tests
	@echo "Running tests..."
	@go test -race -timeout 30s ./...

test-verbose: ## Run tests with verbose output
	@echo "Running tests (verbose)..."
	@go test -v -race -timeout 30s ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...
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

run: build ## Build and run with example directory
	@echo "Running $(BINARY_NAME) on current directory..."
	@$(BUILD_DIR)/$(BINARY_NAME) extract .

dev: ## Run in development mode (no build cache)
	@echo "Running in dev mode..."
	@go run $(BINARY_PATH) extract .

deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "Dependencies ready"

tidy: ## Tidy go.mod and go.sum
	@echo "Tidying modules..."
	@go mod tidy
	@echo "Modules tidied"
