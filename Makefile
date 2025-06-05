# Makefile for ironic-metadata

BINARY_NAME=ironic-metadata
BINARY_PATH=./cmd/ironic-metadata
BUILD_DIR=./bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the application
build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(BINARY_PATH)

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application
run:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH)
	$(BUILD_DIR)/$(BINARY_NAME)

# Run with development settings
run-dev:
	IRONIC_URL=http://localhost:6385 \
	BIND_ADDR=127.0.0.1 \
	BIND_PORT=8080 \
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(BINARY_PATH) && \
	$(BUILD_DIR)/$(BINARY_NAME)

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code
lint:
	golangci-lint run

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Install dependencies for development
dev-deps:
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint

# Create a release
release: clean build-all
	@echo "Creating release artifacts..."
	tar -czf $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64
	tar -czf $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64
	zip -j $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe

# Docker build
docker-build:
	docker build -t ironic-metadata:latest .

# Docker run
docker-run:
	docker run --rm -p 8080:80 -e IRONIC_URL=http://host.docker.internal:6385 ironic-metadata:latest

# Help
help:
	@echo "Available commands:"
	@echo "  build        - Build the application"
	@echo "  build-all    - Build for all platforms"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  deps         - Download dependencies"
	@echo "  run          - Build and run the application"
	@echo "  run-dev      - Run with development settings"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  dev-deps     - Install development dependencies"
	@echo "  release      - Create release artifacts"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  help         - Show this help"

.PHONY: build build-all build-linux build-darwin build-windows clean test test-coverage deps run run-dev fmt lint dev-deps release docker-build docker-run help
