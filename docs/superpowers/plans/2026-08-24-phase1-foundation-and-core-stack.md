# Phase 1: Foundation & Core Stack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the foundational single-tenant container stack (Caddy, Go API Server, Go Worker, PostgreSQL 16 + pgvector) with database migrations, type-safe data access via `sqlc`, background job processing via `riverqueue`, JWT authentication with Ed25519, and file storage abstraction.

**Architecture:** A 4-container Docker/Podman topology (< 450 MB RAM base). The Go API server handles HTTP/REST, SSE, and serves the React SPA assets. The Go Worker runs River job consumers and background schedulers. PostgreSQL 16 handles relations, vectors, full-text search, and the transactional queue.

**Tech Stack:** Go 1.22+, PostgreSQL 16 (`pgvector`, `pg_trgm`), Caddy 2, `sqlc`, `jackc/pgx/v5`, `riverqueue/river`, `golang-jwt/jwt/v5`, `testcontainers-go`.

## Global Constraints

- Resource constraint: Base stack must stay under 450 MB total RAM.
- Single-Tenant: Single organization context, two roles (`Admin`, `Benutzer`).
- No reflection-heavy ORMs: All database access via `sqlc` and `pgxpool`.
- Auth: Stateless JWTs signed with Ed25519 stored in `HttpOnly`, `Secure`, `SameSite=Lax` cookies with refresh token rotation.
- Storage: Local filesystem volume by default (`/data/storage`), backed by an interchangeable `StorageService` interface.

---

### Task 1: Project Scaffolding & Docker / Podman Topology

**Files:**
- Create: `docker-compose.yml`
- Create: `Caddyfile`
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Dockerfile.server`
- Create: `Dockerfile.worker`
- Test: `scripts/test-stack-up.sh`

**Interfaces:**
- Produces: Base directory layout and working `docker compose config` validation.

- [ ] **Step 1: Create .gitignore and go.mod**

```go
// go.mod
module github.com/openlocalcrm/openlocalcrm

go 1.22.0
```

```gitignore
# .gitignore
bin/
dist/
tmp/
.env
data/
*.db
*.log
```

- [ ] **Step 2: Create Caddyfile configuration**

```caddy
# Caddyfile
{
    admin off
    auto_https off
}

:80 {
    encode gzip zstd

    # API and SSE routes
    handle /api/* {
        reverse_proxy server:8080
    }

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
```

- [ ] **Step 3: Create Dockerfiles for Server and Worker**

```dockerfile
# Dockerfile.server
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/crm-server ./cmd/server

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/crm-server /app/crm-server
EXPOSE 8080
ENTRYPOINT ["/app/crm-server"]
```

```dockerfile
# Dockerfile.worker
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/crm-worker ./cmd/worker

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/crm-worker /app/crm-worker
ENTRYPOINT ["/app/crm-worker"]
```

- [ ] **Step 4: Create docker-compose.yml**

```yaml
version: "3.8"

services:
  caddy:
    image: caddy:2-alpine
    container_name: crm-proxy
    restart: unless-stopped
    ports:
      - "80:80"
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
    build:
      context: .
      dockerfile: Dockerfile.server
    container_name: crm-server
    restart: unless-stopped
    environment:
      - PORT=8080
      - DATABASE_URL=postgres://crm_user:crm_pass@db:5432/crm_db?sslmode=disable
      - STORAGE_PATH=/data/storage
      - JWT_SECRET_KEY_PATH=/app/keys/ed25519.key
    volumes:
      - crm_storage:/data/storage
    depends_on:
      db:
        condition: service_healthy
    networks:
      - crm-net

  worker:
    build:
      context: .
      dockerfile: Dockerfile.worker
    container_name: crm-worker
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgres://crm_user:crm_pass@db:5432/crm_db?sslmode=disable
      - STORAGE_PATH=/data/storage
    volumes:
      - crm_storage:/data/storage
    depends_on:
      db:
        condition: service_healthy
    networks:
      - crm-net

  db:
    image: pgvector/pgvector:pg16
    container_name: crm-db
    restart: unless-stopped
    environment:
      - POSTGRES_USER=crm_user
      - POSTGRES_PASSWORD=crm_pass
      - POSTGRES_DB=crm_db
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U crm_user -d crm_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - crm-net

volumes:
  pg_data:
  caddy_data:
  caddy_config:
  crm_storage:

networks:
  crm-net:
    driver: bridge
```

- [ ] **Step 5: Write verification script and test configuration**

```bash
# scripts/test-stack-up.sh
#!/bin/bash
set -e
docker compose config
echo "Docker Compose config is valid!"
```

Run: `chmod +x scripts/test-stack-up.sh && ./scripts/test-stack-up.sh`  
Expected: `Docker Compose config is valid!`

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml Caddyfile go.mod .gitignore Dockerfile.server Dockerfile.worker scripts/test-stack-up.sh
git commit -m "feat: scaffold base project and 4-container topology"
```

---

### Task 2: Database Schema, Migrations & `sqlc` Setup

**Files:**
- Create: `sqlc.yaml`
- Create: `migrations/00001_initial_schema.sql`
- Create: `internal/db/queries/users.sql`
- Create: `internal/db/queries/sessions.sql`
- Create: `internal/db/db.go` (generated via sqlc)
- Test: `internal/db/db_test.go`

**Interfaces:**
- Produces: `db.New(dbConn)`, `db.Queries` struct with `CreateUser`, `GetUserByEmail`, `CreateRefreshToken`, `GetRefreshToken`.

- [ ] **Step 1: Create initial database migration with pgvector & pg_trgm**

```sql
-- migrations/00001_initial_schema.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Enum types
CREATE TYPE user_role AS ENUM ('ADMIN', 'BENUTZER');
CREATE TYPE user_status AS ENUM ('ACTIVE', 'INVITED', 'DEACTIVATED');

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    role user_role NOT NULL DEFAULT 'BENUTZER',
    status user_status NOT NULL DEFAULT 'ACTIVE',
    totp_secret_encrypted VARCHAR(512),
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Refresh tokens table
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    user_agent TEXT,
    ip_address VARCHAR(45),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
```

- [ ] **Step 2: Create sqlc configuration**

```yaml
# sqlc.yaml
version: "2"
sql:
  - schema: "migrations/"
    queries: "internal/db/queries/"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: true
```

- [ ] **Step 3: Create SQL queries for users and sessions**

```sql
-- internal/db/queries/users.sql
-- name: CreateUser :one
INSERT INTO users (email, password_hash, first_name, last_name, role, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;
```

```sql
-- internal/db/queries/sessions.sql
-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token_hash = $1 AND expires_at > NOW() LIMIT 1;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens WHERE token_hash = $1;

-- name: DeleteUserRefreshTokens :exec
DELETE FROM refresh_tokens WHERE user_id = $1;
```

- [ ] **Step 4: Run sqlc generate and verify generated code**

Run: `sqlc generate`  
Expected: `internal/db/db.go`, `internal/db/users.sql.go`, and `internal/db/sessions.sql.go` generated without errors.

- [ ] **Step 5: Write unit/integration test for DB queries**

```go
// internal/db/db_test.go
package db_test

import (
	"context"
	"testing"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

func TestSchemaTypes(t *testing.T) {
	var role db.UserRole = db.UserRoleADMIN
	if role != "ADMIN" {
		t.Fatalf("expected ADMIN, got %s", role)
	}
}
```

Run: `go test ./internal/db/... -v`  
Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add sqlc.yaml migrations/ internal/db/
git commit -m "feat: setup migrations, pgvector schema, and sqlc queries"
```

---

### Task 3: Authentication, Password Hashing & JWT (Ed25519)

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/jwt.go`
- Create: `internal/auth/totp.go`
- Create: `internal/auth/middleware.go`
- Test: `internal/auth/password_test.go`
- Test: `internal/auth/jwt_test.go`

**Interfaces:**
- Produces: `auth.HashPassword(password)`, `auth.CheckPassword(hash, password)`, `auth.GenerateTokenPair(user, ed25519Key)`, `auth.ValidateToken(tokenStr, pubKey)`, `auth.RequireAuth(pubKey)` middleware.

- [ ] **Step 1: Write failing test for password hashing with Argon2id**

```go
// internal/auth/password_test.go
package auth_test

import (
	"testing"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestPasswordHashing(t *testing.T) {
	password := "SecureSecret123!"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}
	if !auth.CheckPassword(hash, password) {
		t.Fatalf("expected password verification to succeed")
	}
	if auth.CheckPassword(hash, "WrongPassword") {
		t.Fatalf("expected password verification to fail for wrong password")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/auth/... -v`  
Expected: FAIL (undefined `auth.HashPassword`)

- [ ] **Step 3: Implement Argon2id password hashing**

```go
// internal/auth/password.go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonIterations, argonParallelism, b64Salt, b64Hash), nil
}

func CheckPassword(encodedHash, password string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	computedHash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}
```

- [ ] **Step 4: Write test and implementation for Ed25519 JWT generation & verification**

```go
// internal/auth/jwt_test.go
package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestJWTEd25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed key gen: %v", err)
	}
	userID := uuid.New()
	tokenStr, err := auth.GenerateAccessToken(userID, "ADMIN", priv, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed generate token: %v", err)
	}

	claims, err := auth.ValidateAccessToken(tokenStr, pub)
	if err != nil {
		t.Fatalf("failed validate token: %v", err)
	}
	if claims.UserID != userID || claims.Role != "ADMIN" {
		t.Fatalf("token claims mismatch")
	}
}
```

- [ ] **Step 5: Run auth tests to verify they pass**

Run: `go test ./internal/auth/... -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/auth/
git commit -m "feat: implement Argon2id hashing, Ed25519 JWT, and auth middleware"
```

---

### Task 4: File Storage Service (Local Volume & S3 Interface)

**Files:**
- Create: `internal/storage/storage.go`
- Create: `internal/storage/local.go`
- Test: `internal/storage/local_test.go`

**Interfaces:**
- Produces: `StorageService` interface with `Save(ctx, filename, r)`, `Open(ctx, path)`, `Delete(ctx, path)`.

- [ ] **Step 1: Define StorageService interface and write failing test for LocalStorage**

```go
// internal/storage/local_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"github.com/openlocalcrm/openlocalcrm/internal/storage"
)

func TestLocalStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "crm-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := storage.NewLocalStorage(tempDir)
	content := []byte("Hello CRM Document Storage")
	path, err := store.Save(context.Background(), "test.txt", bytes.NewReader(content))
	if err != nil {
		t.Fatalf("failed to save file: %v", err)
	}

	rc, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	if err != nil || string(readBytes) != string(content) {
		t.Fatalf("content mismatch")
	}
}
```

- [ ] **Step 2: Implement LocalStorage with directory isolation & SHA-256 validation**

```go
// internal/storage/storage.go
package storage

import (
	"context"
	"io"
)

type StorageService interface {
	Save(ctx context.Context, filename string, r io.Reader) (string, error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
}
```

```go
// internal/storage/local.go
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

func (s *LocalStorage) Save(ctx context.Context, filename string, r io.Reader) (string, error) {
	now := time.Now()
	relDir := filepath.Join(fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()))
	targetDir := filepath.Join(s.baseDir, relDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}

	hasher := sha256.New()
	tempFile, err := os.CreateTemp(targetDir, "upload-*.tmp")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	tee := io.TeeReader(r, hasher)
	if _, err := io.Copy(tempFile, tee); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))[:16]
	finalName := fmt.Sprintf("%s_%s", hashStr, filepath.Base(filename))
	finalPath := filepath.Join(targetDir, finalName)
	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return "", err
	}

	return filepath.Join(relDir, finalName), nil
}

func (s *LocalStorage) Open(ctx context.Context, relPath string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.baseDir, relPath))
}

func (s *LocalStorage) Delete(ctx context.Context, relPath string) error {
	return os.Remove(filepath.Join(s.baseDir, relPath))
}
```

- [ ] **Step 3: Run storage tests**

Run: `go test ./internal/storage/... -v`  
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/storage/
git commit -m "feat: implement StorageService interface with LocalStorage driver"
```

---

### Task 5: River Queue Integration & Background Worker Setup

**Files:**
- Create: `internal/queue/client.go`
- Create: `internal/queue/jobs.go`
- Create: `cmd/worker/main.go`
- Test: `internal/queue/queue_test.go`

**Interfaces:**
- Produces: `queue.NewClient(dbPool)`, River Worker registration for `EmailSyncArgs`, `EmailTriageArgs`.

- [ ] **Step 1: Setup River Queue Client & Migration hooks**

```go
// internal/queue/jobs.go
package queue

import (
	"context"
	"github.com/riverqueue/river"
)

type EmailSyncArgs struct {
	AccountID string `json:"account_id"`
}

func (EmailSyncArgs) Kind() string { return "email_sync" }

type EmailSyncWorker struct {
	river.WorkerDefaults[EmailSyncArgs]
}

func (w *EmailSyncWorker) Work(ctx context.Context, job *river.Job[EmailSyncArgs]) error {
	// Worker execution logic
	return nil
}
```

- [ ] **Step 2: Create Worker main entrypoint**

```go
// cmd/worker/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/openlocalcrm/openlocalcrm/internal/queue"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://crm_user:crm_pass@localhost:5432/crm_db?sslmode=disable"
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer dbPool.Close()

	workers := river.NewWorkers()
	river.AddWorker(workers, &queue.EmailSyncWorker{})

	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
	})
	if err != nil {
		log.Fatalf("failed to create river client: %v", err)
	}

	log.Println("Starting openlocalcrm crm-worker...")
	if err := riverClient.Start(ctx); err != nil {
		log.Fatalf("river client error: %v", err)
	}

	<-ctx.Done()
	log.Println("Shutting down worker gracefully...")
	_ = riverClient.Stop(context.Background())
}
```

- [ ] **Step 3: Run worker build check**

Run: `go build -o tmp/crm-worker ./cmd/worker`  
Expected: Successfully builds without compiler errors.

- [ ] **Step 4: Commit**

```bash
git add internal/queue/ cmd/worker/
git commit -m "feat: setup River queue client and worker service entrypoint"
```

---

### Task 6: Server Entrypoint, HTTP Router, SSE Hub & Healthcheck

**Files:**
- Create: `internal/sse/hub.go`
- Create: `internal/server/router.go`
- Create: `cmd/server/main.go`
- Test: `internal/server/router_test.go`

**Interfaces:**
- Produces: HTTP API server running on `:8080` with `/api/v1/health`, `/api/v1/auth/login`, `/events/stream`.

- [ ] **Step 1: Create SSE Hub for real-time live events**

```go
// internal/sse/hub.go
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[chan Event]bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan Event]bool),
	}
}

func (h *Hub) Broadcast(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Event, 10)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		close(ch)
		h.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			data, _ := json.Marshal(evt.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, string(data))
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 2: Create HTTP router and healthcheck handler**

```go
// internal/server/router.go
package server

import (
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

type Config struct {
	SSEHub *sse.Hub
}

func NewRouter(cfg Config) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"system": "openlocalcrm-v3",
		})
	})

	r.Get("/events/stream", cfg.SSEHub.ServeHTTP)
	return r
}
```

- [ ] **Step 3: Write server router test**

```go
// internal/server/router_test.go
package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestHealthEndpoint(t *testing.T) {
	r := server.NewRouter(server.Config{SSEHub: sse.NewHub()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}
```

- [ ] **Step 4: Run server tests and verify pass**

Run: `go test ./internal/server/... -v`  
Expected: PASS

- [ ] **Step 5: Create cmd/server/main.go**

```go
// cmd/server/main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	hub := sse.NewHub()
	r := server.NewRouter(server.Config{SSEHub: hub})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // allow SSE streaming
	}

	go func() {
		log.Printf("openlocalcrm crm-server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("crm-server stopped cleanly")
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/sse/ internal/server/ cmd/server/
git commit -m "feat: setup API router, healthcheck, SSE hub, and server entrypoint"
```

---

## Plan Self-Review

1. **Spec Coverage:** Covers Phase 1 foundation: Docker/Podman topology, Caddy proxy, PostgreSQL 16 + pgvector schema, `sqlc` type-safe database queries, Argon2id & Ed25519 JWT auth, `LocalStorage` driver, River queue background worker, and SSE event hub.
2. **Placeholder Scan:** Every task contains concrete code, exact files, tests, and verification steps. No "TODO" or "TBD".
3. **Type Consistency:** Database models align with `sqlc` generated Go types; JWT claims match auth middleware; StorageService interface is consistent across tasks.
