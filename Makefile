.PHONY: help generate build test clean run lint

# Version info (typically from git tags)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Directories
OUTPUT_DIR = bin
SCHEMA_DIR = schemas
CONFIG_DIR = config/nodes/stella-PowerEdge-T420

# Binary names
CLIENT_BINARY = power-edge-client
SERVER_BINARY = power-edge-server

# Build flags
LDFLAGS = -X main.Version=$(VERSION) \
          -X main.GitCommit=$(GIT_COMMIT) \
          -X main.BuildTime=$(BUILD_TIME)

help: ## Show this help message
	@echo "Power Edge - Edge State Management System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

generate: ## Generate Go code from schemas
	@echo "🔄 Generating code from schemas..."
	go generate ./pkg/config
	@echo "✅ Code generation complete"

build: build-client build-server ## Build all binaries

build-client: generate ## Build the client binary
	@echo "🔨 Building $(CLIENT_BINARY)..."
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(CLIENT_BINARY) \
		./cmd/power-edge-client
	@echo "✅ Build complete: $(OUTPUT_DIR)/$(CLIENT_BINARY)"
	@$(OUTPUT_DIR)/$(CLIENT_BINARY) -version

build-server: ## Build the server binary
	@echo "🔨 Building $(SERVER_BINARY)..."
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(SERVER_BINARY) \
		./cmd/power-edge-server
	@echo "✅ Build complete: $(OUTPUT_DIR)/$(SERVER_BINARY)"

test: ## Run tests
	@echo "🧪 Running tests..."
	go test -v -race -cover ./...

lint: ## Run linters
	@echo "🔍 Running linters..."
	go vet ./...
	go fmt ./...

clean: ## Clean build artifacts
	@echo "🧹 Cleaning..."
	rm -rf $(OUTPUT_DIR)
	rm -f pkg/config/generated.go
	rm -rf /tmp/power-edge-init-*

run: build-client ## Build and run client locally
	@echo "🚀 Running $(CLIENT_BINARY)..."
	$(OUTPUT_DIR)/$(CLIENT_BINARY) \
		-state-config=$(CONFIG_DIR)/state.yaml \
		-watcher-config=$(CONFIG_DIR)/watcher.yaml \
		-listen=:9100 \
		-check-interval=30s

run-dev: ## Run client in development mode (without building)
	@echo "🚀 Running client in dev mode..."
	go run -ldflags "$(LDFLAGS)" ./cmd/power-edge-client \
		-state-config=$(CONFIG_DIR)/state.yaml \
		-watcher-config=$(CONFIG_DIR)/watcher.yaml \
		-listen=:9100 \
		-check-interval=30s

docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t power-edge/power-edge:$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-f Dockerfile .

init: ## Initialize remote node configuration (requires passwordless SSH)
	@echo "🔍 Initializing remote node configuration..."
	@SSH_TARGET="$(SSH)"; \
	if [ -z "$$SSH_TARGET" ]; then \
		echo "Usage: make init SSH=user@hostname"; \
		echo "   Or: export SSH=user@hostname && make init"; \
		echo "Example: make init SSH=stella@10.8.0.1"; \
		echo ""; \
		echo "❌ Error: SSH not set"; \
		exit 1; \
	fi; \
	bash scripts/init/init-node.sh "$$SSH_TARGET"

install: build ## Install binaries to /usr/local/bin (local machine)
	@echo "📦 Installing binaries..."
	sudo cp $(OUTPUT_DIR)/$(CLIENT_BINARY) /usr/local/bin/
	sudo cp $(OUTPUT_DIR)/$(SERVER_BINARY) /usr/local/bin/
	@echo "✅ Installed:"
	@echo "   /usr/local/bin/$(CLIENT_BINARY)"
	@echo "   /usr/local/bin/$(SERVER_BINARY)"

deploy: ## Deploy client to remote node via SSH (auto-detects platform, optional: passwordless sudo or SUDO_PASS)
	@echo "🚀 Deploying to remote node..."
	@# Load .env if it exists
	@set -a; \
	if [ -f .env ]; then \
		echo "   Loading environment from .env"; \
		. ./.env; \
	fi; \
	set +a; \
	SSH_TARGET="$${SSH:-$(SSH)}"; \
	NODE_CFG="$${NODE_CONFIG:-$(NODE_CONFIG)}"; \
	if [ -z "$$SSH_TARGET" ]; then \
		echo "Usage: make deploy SSH=user@hostname NODE_CONFIG=config/nodes/hostname"; \
		echo "   Or: export SSH=user@hostname && make deploy NODE_CONFIG=..."; \
		echo "   Or: Create .env file (see .env.example)"; \
		echo "Example: make deploy SSH=stella@10.8.0.1 NODE_CONFIG=config/nodes/stella-PowerEdge-T420"; \
		echo ""; \
		echo "❌ Error: SSH not set"; \
		exit 1; \
	fi; \
	if [ -z "$$NODE_CFG" ]; then \
		echo ""; \
		echo "❌ Error: NODE_CONFIG not set"; \
		echo "   Run: make deploy SSH=user@hostname NODE_CONFIG=config/nodes/hostname"; \
		exit 1; \
	fi; \
	SUDO_PASS="$${SUDO_PASS:-}" bash scripts/deploy/install-remote.sh "$$SSH_TARGET" "$$NODE_CFG"

version: ## Show version info
	@echo "Version:    $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

# Version management
BUMP ?= patch  # Default to patch bump (patch, minor, major)

bump-patch: ## Bump patch version (0.0.x)
	@$(MAKE) bump BUMP=patch

bump-minor: ## Bump minor version (0.x.0)
	@$(MAKE) bump BUMP=minor

bump-major: ## Bump major version (x.0.0)
	@$(MAKE) bump BUMP=major

bump: ## Bump version (BUMP=patch|minor|major)
	@echo "🏷️  Bumping $(BUMP) version..."
	@CURRENT=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	echo "Current version: $$CURRENT"; \
	bash scripts/build/validate-version.sh suggest $(BUMP) || true
	@echo ""
	@echo "To create release:"
	@echo "  1. Review changes above"
	@echo "  2. Commit your changes"
	@echo "  3. Run: make tag BUMP=$(BUMP)"

tag: ## Create and push version tag (BUMP=patch|minor|major)
	@CURRENT=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	VERSION=$$(echo $$CURRENT | sed 's/v//'); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	if [ "$(BUMP)" = "major" ]; then \
		NEW="v$$((MAJOR + 1)).0.0"; \
	elif [ "$(BUMP)" = "minor" ]; then \
		NEW="v$$MAJOR.$$((MINOR + 1)).0"; \
	else \
		NEW="v$$MAJOR.$$MINOR.$$((PATCH + 1))"; \
	fi; \
	echo "Creating tag: $$NEW"; \
	git tag -a $$NEW -m "Release $$NEW"; \
	echo "✅ Tag created: $$NEW"; \
	echo ""; \
	echo "Push with: git push origin main --tags"
