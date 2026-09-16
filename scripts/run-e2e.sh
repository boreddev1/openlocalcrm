#!/usr/bin/env bash
set -euo pipefail

echo "========================================="
echo "Running Playwright E2E Test Suite"
echo "========================================="

# Ensure Playwright browser binaries are installed
cd web
if ! pnpm exec playwright install chromium --with-deps > /dev/null 2>&1; then
    echo "Installing chromium browser for Playwright..."
    pnpm exec playwright install chromium
fi

echo "Running E2E tests against Vite frontend..."
pnpm exec playwright test --config=../playwright.config.ts

echo "✅ All Playwright E2E UI tests passed successfully!"
