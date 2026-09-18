.PHONY: help build up down restart logs test test-e2e check-licenses run-local clean

help: ## Show help instructions
	@echo "OpenLocalCRM — Single-Tenant Production & Development Commands:"
	@echo "  make build           - Build Web Frontend and Go Server Binary"
	@echo "  make up              - Start complete Single-Tenant Docker stack (Caddy, Server, Worker, Postgres)"
	@echo "  make down            - Stop Docker stack"
	@echo "  make restart         - Restart Docker stack"
	@echo "  make logs            - View live container logs"
	@echo "  make test            - Run all unit and integration tests"
	@echo "  make test-e2e        - Run all 20 Playwright E2E UI tests"
	@echo "  make check-licenses  - Audit 100% Permissive Open Source License Compliance"
	@echo "  make run-local       - Run server locally on :8080"

build-web:
	cd web && pnpm install && pnpm build

build: build-web ## Build Web and Go binary
	go build -ldflags="-w -s" -o bin/crm-server ./cmd/server
	go build -ldflags="-w -s" -o bin/crm-worker ./cmd/worker

GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LAUNCHER_LDFLAGS = -w -s -X "github.com/openlocalcrm/openlocalcrm/internal/launcher.BuildCommit=$(GIT_COMMIT)" -X "github.com/openlocalcrm/openlocalcrm/internal/launcher.BuildDate=$(BUILD_DATE)"

build-windows: ## Build CGO-free Windows Release Binary (no console window)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS) -H=windowsgui" -o bin/openlocalcrm-setup.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup.exe bin/mavalio-setup.exe
	cp bin/openlocalcrm-setup.exe bin/openlocalcrm-setup-windows-amd64.exe

build-windows-debug: ## Build Windows Debug Binary with visible console
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-debug.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup-debug.exe bin/mavalio-setup-debug.exe

build-darwin-arm64: ## Build macOS Apple Silicon binary
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-darwin-arm64 ./cmd/setup-launcher

build-darwin-amd64: ## Build macOS Intel binary
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-darwin-amd64 ./cmd/setup-launcher

build-linux-amd64: ## Build Linux x86_64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-linux-amd64 ./cmd/setup-launcher

build-linux-arm64: ## Build Linux ARM64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-linux-arm64 ./cmd/setup-launcher

build-all-launcher: build-windows build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64 ## Build launcher for all platforms

up: ## Start Docker Compose stack in background
	docker compose up -d --build

down: ## Stop Docker Compose stack
	docker compose down

restart: down up ## Restart Docker stack

logs: ## View real-time container logs
	docker compose logs -f

test: ## Run Go tests
	go test -v ./...

test-e2e: ## Run Playwright E2E tests
	./scripts/run-e2e.sh

fmt: ## Format Go source code with gofmt
	gofmt -s -w .

lint: ## Run Go static analysis, frontend ESLint and TypeScript checks
	go vet ./...
	cd web && pnpm lint && pnpm typecheck

check: setup-hooks fmt lint check-licenses test ## Run full suite of local quality checks

setup-hooks: ## Configure and enable enterprise Git pre-commit and pre-push hooks
	git config core.hooksPath .githooks
	chmod +x .githooks/*
	@echo "✅ Pre-Commit & Pre-Push Hooks erfolgreich eingerichtet!"

simulate: ## Run 10-minute visual live simulation in browser
	cd web && pnpm exec playwright test e2e/live-simulation.spec.ts --headed

check-licenses: ## Verify Open Source License compliance
	./scripts/check-licenses.sh

run-local: build-web ## Run Go binary locally
	go run ./cmd/server

clean: ## Clean build artifacts
	rm -rf bin/ web/dist/
