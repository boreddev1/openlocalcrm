#!/usr/bin/env bash
set -euo pipefail

echo "Validating Docker Compose configuration..."
docker compose config > /dev/null
echo "✅ Docker Compose config is valid!"

echo "Verifying Go builds for server and worker..."
go build -o tmp/crm-server ./cmd/server
go build -o tmp/crm-worker ./cmd/worker
rm -rf tmp/
echo "✅ Go binaries compile cleanly!"
