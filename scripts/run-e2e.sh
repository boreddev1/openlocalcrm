#!/usr/bin/env bash
set -euo pipefail

echo "========================================="
echo "Running Playwright E2E Test Suite"
echo "========================================="

# Ensure working directory is project root
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Build crm-server binary for E2E demo mode execution
echo "Ensuring Go backend server binary is built..."
go build -o bin/crm-server ./cmd/server

# Ensure Playwright browser binaries are installed
cd web
if ! pnpm exec playwright install chromium --with-deps > /dev/null 2>&1; then
    echo "Installing chromium browser for Playwright..."
    pnpm exec playwright install chromium
fi

echo "Running E2E tests against Vite frontend & Go demo backend..."
pnpm exec playwright test --config=../playwright.config.ts

echo "✅ All Playwright E2E UI tests passed successfully!"
