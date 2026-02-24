.PHONY: help run-backend run-gateway run-frontend run-all \
	build-backend build-gateway build-frontend build-sdk build-cli build-installer build-all \
	clean fmt test test-sdk test-cli test-installer deploy-k8s port-forward-k8s

# Output directory
OUTPUT_DIR := $(PWD)/bin

help:
	@echo "LiteBoxd Development Commands"
	@echo ""
	@echo "  make run-backend          - Run backend server"
	@echo "  make run-gateway          - Run gateway server"
	@echo "  make run-frontend         - Run frontend dev server"
	@echo "  make run-all              - Run backend, gateway and frontend"
	@echo "  make build-backend        - Build backend binary"
	@echo "  make build-gateway        - Build gateway binary"
	@echo "  make build-frontend       - Build frontend for production"
	@echo "  make build-sdk            - Build Go SDK"
	@echo "  make build-cli            - Build CLI tool"
	@echo "  make build-installer      - Build one-click installer tool"
	@echo "  make build-all            - Build everything"
	@echo "  make clean                - Clean build artifacts"
	@echo "  make fmt                  - Format code"
	@echo "  make test                 - Run tests"
	@echo "  make test-sdk             - Run SDK tests"
	@echo "  make test-cli             - Run CLI tests"
	@echo "  make test-installer       - Run installer tests"
	@echo "  make deploy-k8s           - Deploy k8s manifests (REGISTRY required, TAG optional)"
	@echo "  make port-forward-k8s     - One-command port-forward for API and gateway"

# Development
run-backend:
	cd backend && go run ./cmd/server

run-gateway:
	cd backend && go run ./cmd/gateway

run-frontend:
	cd web && npm run dev

run-all:
	@echo "Starting backend, gateway and frontend..."
	@make run-backend &
	@make run-gateway &
	@make run-frontend

# Build
build-backend:
	@echo "Building backend..."
	@mkdir -p $(OUTPUT_DIR)
	cd backend && go build -o $(OUTPUT_DIR)/liteboxd-server ./cmd/server
	@echo "Backend built: $(OUTPUT_DIR)/liteboxd-server"

build-gateway:
	@echo "Building gateway..."
	@mkdir -p $(OUTPUT_DIR)
	cd backend && go build -o $(OUTPUT_DIR)/liteboxd-gateway ./cmd/gateway
	@echo "Gateway built: $(OUTPUT_DIR)/liteboxd-gateway"

build-frontend:
	@echo "Building frontend..."
	cd web && npm run build
	@echo "Frontend built: web/dist/"

build-sdk:
	@echo "Building Go SDK..."
	@mkdir -p $(OUTPUT_DIR)
	cd sdk/go && go build ./...
	@echo "SDK built successfully"

build-cli:
	@echo "Building CLI tool..."
	@mkdir -p $(OUTPUT_DIR)
	cd liteboxd-cli && go build -o $(OUTPUT_DIR)/liteboxd .
	@echo "CLI built: $(OUTPUT_DIR)/liteboxd"

build-installer:
	@echo "Building installer tool..."
	@mkdir -p $(OUTPUT_DIR)
	cd tools/liteboxd-installer && GOCACHE=/tmp/go-build go build -o $(OUTPUT_DIR)/liteboxd-installer .
	@echo "Installer built: $(OUTPUT_DIR)/liteboxd-installer"

build-all: build-backend build-gateway build-cli build-installer
	@echo "All builds complete!"

# Tests
test:
	@echo "Running all tests..."
	cd backend && go test ./...
	cd sdk/go && go test ./...

test-sdk:
	@echo "Running SDK tests..."
	cd sdk/go && go test ./...

test-cli:
	@echo "Running CLI tests..."
	cd liteboxd-cli && go test ./...

test-installer:
	@echo "Running installer tests..."
	cd tools/liteboxd-installer && GOCACHE=/tmp/go-build go test ./...

# Deployment
deploy-k8s:
	@echo "Deploying k8s manifests..."
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Usage: make deploy-k8s REGISTRY=<registry> [TAG=<tag>]"; \
		exit 1; \
	fi
	REGISTRY="$(REGISTRY)" TAG="$(TAG)" bash deploy/scripts/deploy-k8s.sh

port-forward-k8s:
	@echo "Starting k8s port-forward..."
	bash deploy/scripts/port-forward-k8s.sh

# Clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)
	rm -rf backend/bin
	rm -rf web/dist
	@echo "Clean complete!"

# Format
fmt:
	@echo "Formatting code..."
	cd backend && go fmt ./...
	cd sdk/go && go fmt ./...
	cd liteboxd-cli && go fmt ./...
	cd tools/liteboxd-installer && go fmt ./...
	cd web && npm run format 2>/dev/null || true
	@echo "Formatting complete!"
