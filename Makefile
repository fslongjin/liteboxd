.PHONY: help run-backend run-frontend run-all stop-k3s start-k3s \
	build-backend build-frontend build-sdk build-cli build-all \
 clean fmt test test-sdk test-cli

# Output directory
OUTPUT_DIR := $(PWD)/bin

help:
	@echo "LiteBoxd Development Commands"
	@echo ""
	@echo "  make start-k3s      - Start k3s in Docker"
	@echo "  make stop-k3s       - Stop k3s"
	@echo "  make run-backend    - Run backend server"
	@echo "  make run-frontend   - Run frontend dev server"
	@echo "  make run-all        - Run both backend and frontend"
	@echo "  make build-backend  - Build backend binary"
	@echo "  make build-frontend - Build frontend for production"
	@echo "  make build-sdk      - Build Go SDK"
	@echo "  make build-cli      - Build CLI tool"
	@echo "  make build-all      - Build everything"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make fmt            - Format code"
	@echo "  make test           - Run tests"
	@echo "  make test-sdk       - Run SDK tests"
	@echo "  make test-cli       - Run CLI tests"

# K3s management
start-k3s:
	cd deploy && docker-compose up -d
	@echo "Waiting for kubeconfig..."
	@until [ -f deploy/kubeconfig/kubeconfig.yaml ]; do sleep 2; done
	@echo "K3s is ready!"

stop-k3s:
	cd deploy && docker-compose down

# Development
run-backend:
	@export KUBECONFIG=$(PWD)/deploy/kubeconfig/kubeconfig.yaml && \
	cd backend && go run ./cmd/server

run-frontend:
	cd web && npm run dev

run-all:
	@echo "Starting backend and frontend..."
	@make run-backend &
	@make run-frontend

# Build
build-backend:
	@echo "Building backend..."
	@mkdir -p $(OUTPUT_DIR)
	cd backend && go build -o $(OUTPUT_DIR)/liteboxd-server ./cmd/server
	@echo "Backend built: $(OUTPUT_DIR)/liteboxd-server"

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

build-all: build-backend build-cli
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
	cd web && npm run format 2>/dev/null || true
	@echo "Formatting complete!"
