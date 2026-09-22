package launcher

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultDockerCompose = `name: openlocalcrm

services:
  caddy:
    image: caddy:2-alpine
    container_name: crm-proxy
    restart: unless-stopped
    ports:
      - "${APP_PORT:-80}:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      - server
    networks:
      - crm-net

  server:
    image: ${CRM_SERVER_IMAGE:-crm-server:local}
    build:
      context: .
      dockerfile: Dockerfile.server
    container_name: crm-server
    restart: unless-stopped
    environment:
      - PORT=8080
      - DATABASE_URL=${DATABASE_URL:-postgres://postgres:crm_pass@db:5432/openlocalcrm?sslmode=disable}
      - STORAGE_PATH=/data/storage
      - JWT_SECRET_KEY_PATH=/app/keys/ed25519.key
      - DEMO_MODE=${DEMO_MODE:-false}
      - INITIAL_ADMIN_EMAIL=${INITIAL_ADMIN_EMAIL:-admin@openlocalcrm.local}
      - INITIAL_ADMIN_PASSWORD=${INITIAL_ADMIN_PASSWORD:-}
      - AI_PROVIDER=${AI_PROVIDER:-ollama}
      - OLLAMA_BASE_URL=${OLLAMA_BASE_URL:-http://host.docker.internal:11434}
      - OLLAMA_MODEL=${OLLAMA_MODEL:-gemma4:12b}
      - AI_EMBEDDING_MODEL=${AI_EMBEDDING_MODEL:-}
      - OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-}
      - AI_API_KEY=${AI_API_KEY:-}
      - AI_BASE_URL=${AI_BASE_URL:-}
      - CONNECTOR_API_TOKEN=${CONNECTOR_API_TOKEN:-}
    volumes:
      - crm_storage:/data/storage
      - crm_keys:/app/keys
    depends_on:
      db:
        condition: service_healthy
    networks:
      - crm-net

  worker:
    image: ${CRM_WORKER_IMAGE:-crm-worker:local}
    build:
      context: .
      dockerfile: Dockerfile.worker
    container_name: crm-worker
    restart: unless-stopped
    environment:
      - DATABASE_URL=${DATABASE_URL:-postgres://postgres:crm_pass@db:5432/openlocalcrm?sslmode=disable}
      - STORAGE_PATH=/data/storage
    volumes:
      - crm_storage:/data/storage
    depends_on:
      db:
        condition: service_healthy
      server:
        condition: service_started
    networks:
      - crm-net

  db:
    image: pgvector/pgvector:pg16
    container_name: crm-db
    restart: unless-stopped
    environment:
      - POSTGRES_USER=${DB_USER:-postgres}
      - POSTGRES_PASSWORD=${DB_PASSWORD:-crm_pass}
      - POSTGRES_DB=${DB_NAME:-openlocalcrm}
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER:-postgres} -d ${DB_NAME:-openlocalcrm}"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - crm-net

volumes:
  pg_data:
    name: crm_pg_data
  caddy_data:
    name: crm_caddy_data
  caddy_config:
    name: crm_caddy_config
  crm_storage:
    name: crm_storage
  crm_keys:
    name: crm_keys

networks:
  crm-net:
    driver: bridge
`

const defaultCaddyfile = `# Caddyfile for OpenLocalCRM (Single-Tenant v3)
{
    admin off
}

:80, :443 {
    encode gzip zstd

    # HTTP Security Headers
    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
        X-XSS-Protection "1; mode=block"
        Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' http://localhost:8080 http://localhost:11434 http://127.0.0.1:11434 http://host.docker.internal:11434; img-src 'self' data: https:; font-src 'self' data:; object-src 'none'; base-uri 'self';"
    }

    # API routes
    handle /api/* {
        reverse_proxy server:8080
    }

    # SSE streaming route with immediate flush
    handle /events/* {
        reverse_proxy server:8080 {
            flush_interval -1
        }
    }

    # Static assets and SPA fallback
    handle {
        reverse_proxy server:8080
    }
}
`

// EnsureComposeAndCaddyFiles checks if docker-compose.yml and Caddyfile exist in baseDir.
// If not, it writes the embedded defaults.
func EnsureComposeAndCaddyFiles(baseDir string) error {
	composePath := filepath.Join(baseDir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		if err := os.WriteFile(composePath, []byte(defaultDockerCompose), 0644); err != nil {
			return fmt.Errorf("failed to write default docker-compose.yml: %w", err)
		}
	}

	caddyPath := filepath.Join(baseDir, "Caddyfile")
	if _, err := os.Stat(caddyPath); os.IsNotExist(err) {
		if err := os.WriteFile(caddyPath, []byte(defaultCaddyfile), 0644); err != nil {
			return fmt.Errorf("failed to write default Caddyfile: %w", err)
		}
	}

	return nil
}

// PatchComposeEmbeddingDefaults replaces embedding env lines that hardcode a
// qwen3 compose default with the empty-default form. Missing lines stay
// missing — absence is the safe state (the gateway then falls back to its own
// provider-scoped default or the honest config error).
func PatchComposeEmbeddingDefaults(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if strings.Contains(line, "OLLAMA_EMBEDDING_MODEL=") && strings.Contains(line, ":-qwen3-embedding:0.6b}") {
			lines[i] = strings.Replace(line, ":-qwen3-embedding:0.6b}", ":-}", 1)
			changed = true
		}
		if strings.Contains(line, "AI_EMBEDDING_MODEL=") && strings.Contains(line, ":-qwen3-embedding:0.6b}") {
			lines[i] = strings.Replace(line, ":-qwen3-embedding:0.6b}", ":-}", 1)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}
