# Variables
BINARY_NAME=canhazgpu
BUILD_DIR=build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# help text
.PHONY: help
help:
	@echo "Go build targets:"
	@echo "  make build        - Build the canhazgpu binary"
	@echo "  make install      - Build and install to /usr/local/bin"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make test         - Run tests (includes integration tests - may be slow)"
	@echo "  make test-short   - Run tests (skip integration tests - fast)"
	@echo "  make test-coverage- Run tests with coverage report"
	@echo "  make test-integration - Run integration tests only (requires Redis/nvidia-smi)"
	@echo "  make deps         - Download Go dependencies"
	@echo ""
	@echo "Documentation targets:"
	@echo "  make docs-deps    - Install documentation dependencies"
	@echo "  make docs         - Build documentation with MkDocs"
	@echo "  make docs-preview - Build and serve documentation locally"
	@echo "  make docs-clean   - Clean documentation build files"

.PHONY: deps
deps:
	@echo "Downloading Go dependencies"
	@go mod download
	@go mod tidy

.PHONY: build
build: deps
	@echo "Building $(BINARY_NAME)"
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) main.go

.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin"
	@sudo cp -v $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@sudo cp -v ./autocomplete_canhazgpu.sh /etc/bash_completion.d/autocomplete_canhazgpu.sh

.PHONY: clean
clean:
	@echo "Cleaning build artifacts"
	@rm -rf $(BUILD_DIR)

.PHONY: lint
lint:
	@echo "Running lint"
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint run

.PHONY: fmt
fmt:
	@echo "Running fmt"
	@go fmt ./...

.PHONY: test
test:
	@echo "Running tests"
	@go test -v ./...

.PHONY: test-short
test-short:
	@echo "Running tests (short mode, skipping integration tests)"
	@go test -short -v ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage"
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

.PHONY: test-integration
test-integration:
	@echo "Running integration tests (requires Redis)"
	@go test -v ./... -run Integration

.PHONY: docs-deps
docs-deps:
	@echo "Installing documentation dependencies"
	@pip install -r requirements-docs.txt

.PHONY: docs
docs:
	@echo "Building documentation with MkDocs"
	@command -v mkdocs >/dev/null 2>&1 || { echo "Error: mkdocs not found. Install with: make docs-deps"; exit 1; }
	@mkdocs build || { echo "Error: MkDocs build failed. Install dependencies with: make docs-deps"; exit 1; }

.PHONY: docs-preview
docs-preview:
	@echo "Starting MkDocs development server"
	@command -v mkdocs >/dev/null 2>&1 || { echo "Error: mkdocs not found. Install with: make docs-deps"; exit 1; }
	@echo "Documentation will be available at: http://127.0.0.1:8000"
	@echo "Press Ctrl+C to stop the server"
	@mkdocs serve || { echo "Error: MkDocs serve failed. Install dependencies with: make docs-deps"; exit 1; }

.PHONY: docs-clean
docs-clean:
	@echo "Cleaning documentation build files"
	@rm -rf site/
