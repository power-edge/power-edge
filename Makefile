.PHONY: help generate build test clean run lint

# Version info (typically from git tags)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Directories
OUTPUT_DIR = bin
SCHEMA_DIR = schemas
CONFIG_DIR = config/nodes/stella-PowerEdge-T420
BINARY_NAME = power-edge

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
	go run apps/edge-state-exporter/cmd/generator/main.go \
		-schema-dir=$(SCHEMA_DIR) \
		-output-dir=apps/edge-state-exporter/pkg/config
	@echo "✅ Code generation complete"

build: generate ## Build the power-edge binary
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	go build -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME) \
		apps/edge-state-exporter/cmd/exporter/main.go
	@echo "✅ Build complete: $(OUTPUT_DIR)/$(BINARY_NAME)"
	@$(OUTPUT_DIR)/$(BINARY_NAME) -version

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
	rm -f apps/edge-state-exporter/pkg/config/generated.go
	rm -rf /tmp/power-edge-init-*

run: build ## Build and run locally
	@echo "🚀 Running $(BINARY_NAME)..."
	$(OUTPUT_DIR)/$(BINARY_NAME) \
		-state-config=$(CONFIG_DIR)/state.yaml \
		-watcher-config=$(CONFIG_DIR)/watcher.yaml \
		-listen=:9100 \
		-check-interval=30s

run-dev: ## Run in development mode (without building)
	@echo "🚀 Running in dev mode..."
	go run -ldflags "$(LDFLAGS)" apps/edge-state-exporter/cmd/exporter/main.go \
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
		-f apps/edge-state-exporter/Dockerfile .

init: ## Initialize node configuration from remote server
	@echo "🔍 Initializing node configuration..."
	@echo "Usage: make init SSH_HOST=user@hostname"
	@echo "Example: make init SSH_HOST=stella@10.8.0.1"
	@if [ -z "$(SSH_HOST)" ]; then \
		echo ""; \
		echo "❌ Error: SSH_HOST not set"; \
		echo "   Run: make init SSH_HOST=user@hostname"; \
		exit 1; \
	fi
	bash scripts/probe/init-node.sh $(SSH_HOST)

install: build ## Install binary to /usr/local/bin
	@echo "📦 Installing $(BINARY_NAME)..."
	sudo cp $(OUTPUT_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "✅ Installed to /usr/local/bin/$(BINARY_NAME)"

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
