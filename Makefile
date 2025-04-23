.PHONY: build test lint fmt clean run vet

# Get repository name for binary name
REPO_NAME=$(shell basename $(shell git rev-parse --show-toplevel))
BINARY_NAME=$(REPO_NAME)
BUILD_DIR=./bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOVET=$(GOCMD) vet
GOFMT=gofmt -s -d
GOLINT=golangci-lint

# Build the binary
build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Run the binary
run: build
	$(BUILD_DIR)/$(BINARY_NAME)

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Check code format
fmt:
	$(GOFMT) .

# Apply code formatting changes
fmt-write:
	gofmt -s -w .

# Run go vet
vet:
	$(GOVET) ./...

# Run linter
lint:
	go fmt ./...
	$(GOLINT) run ./...

# Check all (format, lint, vet, test)
check: fmt lint vet test

# Install dependencies
deps:
	$(GOGET) -v ./...
	go mod tidy

# Update dependencies
deps-update:
	go get -u ./...
	go mod tidy

# Build Docker image
docker-build:
	docker build -t $(BINARY_NAME):latest .

# Default target
all: clean build

# Print current configuration
info:
	@echo "Repository name: $(REPO_NAME)"
	@echo "Binary name: $(BINARY_NAME)"
	@echo "Build directory: $(BUILD_DIR)" 
