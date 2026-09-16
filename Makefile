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

build-windows: ## Build CGO-free Windows Release Binary (no console window)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-w -s -H=windowsgui" -o bin/openlocalcrm-setup.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup.exe bin/mavalio-setup.exe

build-windows-debug: ## Build Windows Debug Binary with visible console
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/openlocalcrm-setup-debug.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup-debug.exe bin/mavalio-setup-debug.exe

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

simulate: ## Run 10-minute visual live simulation in browser
	cd web && pnpm exec playwright test e2e/live-simulation.spec.ts --headed

check-licenses: ## Verify Open Source License compliance
	./scripts/check-licenses.sh

run-local: build-web ## Run Go binary locally
	go run ./cmd/server

clean: ## Clean build artifacts
	rm -rf bin/ web/dist/
