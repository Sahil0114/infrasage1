# InfraSage Makefile
# Targets mirror the conventions defined in CONVENTIONS.md

BINARY      := bin/infrasage
MODULE      := github.com/Sahil0114/infrasage1
COMPOSE_FILE := deploy/docker-compose.yml
MODEL       := qwen2.5-coder:3b

.PHONY: all build test lint fmt tidy clean \
        stack-up stack-down stack-status \
        model-pull model-status \
        setup help

# ── Default ────────────────────────────────────────────────────────────────────
all: fmt tidy build

# ── Build ──────────────────────────────────────────────────────────────────────
build:
	@echo "🔨 Building $(BINARY)..."
	@mkdir -p bin
	go build -o $(BINARY) .
	@echo "✅ Build complete → $(BINARY)"

# ── Test ───────────────────────────────────────────────────────────────────────
test:
	@echo "🧪 Running tests..."
	go test ./... -v -count=1
	@echo "✅ Tests complete"

# ── Lint / vet ─────────────────────────────────────────────────────────────────
lint:
	@echo "🔍 Running go vet..."
	go vet ./...
	@echo "✅ go vet passed"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "🔍 Running golangci-lint..."; \
		golangci-lint run ./...; \
	else \
		echo "ℹ  golangci-lint not installed — skipping (install: brew install golangci-lint)"; \
	fi

# ── Format ─────────────────────────────────────────────────────────────────────
fmt:
	@echo "🎨 Formatting Go source..."
	go fmt ./...
	@echo "✅ Format complete"

# ── Tidy ───────────────────────────────────────────────────────────────────────
tidy:
	@echo "📦 Tidying Go modules..."
	go mod tidy
	@echo "✅ go mod tidy complete"

# ── Clean ──────────────────────────────────────────────────────────────────────
clean:
	@echo "🧹 Cleaning build artefacts..."
	rm -rf bin/
	rm -rf /tmp/infrasage-scans
	@echo "✅ Clean complete"

# ── Docker stack ───────────────────────────────────────────────────────────────
stack-up:
	@echo "🐳 Starting InfraSage Docker stack..."
	docker compose -f $(COMPOSE_FILE) up -d
	@echo "✅ Stack up — Grafana: http://localhost:3000  Prometheus: http://localhost:9090"

stack-down:
	@echo "🛑 Stopping InfraSage Docker stack..."
	docker compose -f $(COMPOSE_FILE) down

stack-status:
	docker compose -f $(COMPOSE_FILE) ps

# ── Ollama model management ────────────────────────────────────────────────────
model-pull:
	@echo "⬇  Pulling Ollama model: $(MODEL)"
	ollama pull $(MODEL)
	@echo "✅ Model ready"

model-status:
	ollama list

# ── One-time project setup ─────────────────────────────────────────────────────
# Checks required tools, pulls the model, starts the Docker stack.
setup: _check-deps model-pull stack-up build
	@echo ""
	@echo "🎉 InfraSage is ready!"
	@echo "   Run: ./$(BINARY) ask \"create an S3 bucket with versioning\""

_check-deps:
	@echo "🔎 Checking required dependencies..."
	@command -v go >/dev/null 2>&1 || { echo "❌ Go not found. Install from https://go.dev/dl/"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "❌ Docker not found. Install Docker Desktop from https://docker.com"; exit 1; }
	@command -v terraform >/dev/null 2>&1 || { echo "❌ Terraform not found. Run: brew install terraform"; exit 1; }
	@command -v ollama >/dev/null 2>&1 || { echo "❌ Ollama not found. Run: brew install ollama"; exit 1; }
	@command -v git >/dev/null 2>&1 || { echo "❌ git not found. Run: xcode-select --install"; exit 1; }
	@echo "✅ All required tools found"

# ── Help ───────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "InfraSage — AI-powered DevSecOps CLI"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build targets:"
	@echo "  build        Build the infrasage binary to bin/infrasage"
	@echo "  test         Run all Go tests"
	@echo "  lint         Run go vet (and golangci-lint if installed)"
	@echo "  fmt          Format all Go source files"
	@echo "  tidy         Run go mod tidy"
	@echo "  clean        Remove bin/ and /tmp/infrasage-scans"
	@echo ""
	@echo "Docker targets:"
	@echo "  stack-up     Start scanner + monitoring containers"
	@echo "  stack-down   Stop all containers"
	@echo "  stack-status Show container status"
	@echo ""
	@echo "Ollama targets:"
	@echo "  model-pull   Download the qwen2.5-coder:3b model"
	@echo "  model-status List downloaded models"
	@echo ""
	@echo "Setup:"
	@echo "  setup        One-time setup: check deps, pull model, start stack, build"
	@echo ""
