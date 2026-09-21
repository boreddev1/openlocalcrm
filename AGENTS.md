# AGENTS.md — OpenLocalCRM

Guidance for AI coding agents (and humans) working in this repository. Read this before making changes. When in doubt, prefer the existing pattern over inventing a new one.

## Project Overview

**OpenLocalCRM** is a single-tenant, self-hosted CRM built for German sales teams (energy, direct sales, trades, B2B). It prioritizes data sovereignty (DSGVO / UWG compliant), an AI copilot with active PII guards, reactive workflows with human-in-the-loop, and a tight runtime footprint (**< 420 MB RAM** target).

### Stack

| Layer | Technology |
| --- | --- |
| Backend | Go 1.26, Chi router, `jackc/pgx/v5` |
| Data access | `sqlc` (compiled, typed SQL — **no ORM**) |
| Frontend | React 18, Vite 6, TypeScript, Tailwind CSS 3.4 |
| Frontend delivery | Embedded in the Go binary via `//go:embed` (no Node at runtime) |
| Database | PostgreSQL 16 (`pgvector/pgvector:pg16`), auto-migrations |
| Background jobs | River queue (`riverqueue/river`) |
| Auth | Argon2id passwords, Ed25519 JWTs (HttpOnly), TOTP 2FA, CSRF, rate limiting |
| Realtime | Server-Sent Events (`internal/sse`) |
| Reverse proxy | Caddy v2 (Auto-TLS, SSE unbuffered) |
| E2E | Playwright (Chromium) |

- Go module: `github.com/openlocalcrm/openlocalcrm`
- Git remote: `https://github.com/boreddev1/openlocalcrm.git` (default branch: `main`)
- Runtime processes: `crm-server` (:8080), `crm-worker` (:8081), `crm-db` (:5432), `crm-proxy`/Caddy (:80/:443)

## Repository Layout

```
cmd/
  server/            -> bin/crm-server   (REST API, auth, embedded SPA)
  worker/            -> bin/crm-worker   (River queue daemon)
  setup-launcher/    -> launcher binaries (cross-platform installer/updater)
internal/
  server/            router.go, csrf.go, handlers/ (REST handlers, one per domain)
  core/              business logic per domain: contact, company, deal, email,
                     note, todo, appointment, automation, audit, export,
                     notification, reports, telephony
  db/                sqlc-generated code (querier.go, *.sql.go), migrator.go
    queries/         source .sql queries (edit these, then sqlc generate)
    migrations/      NNNNN_name.sql migrations (embedded into binary)
  auth/              jwt, password, totp, csrf, limiter, middleware, service
  ai/                gateway, triage, guard (PII), research, chat, observability
  queue/             River client + job definitions
  storage/           StorageService (path-safe, SHA-256 dedup)
  sse/               SSE hub for live updates
  connectors/        lead-intake webhook engine
  launcher/          cross-platform setup launcher (BuildCommit/BuildDate)
web/                 React SPA (src/{api,context,hooks,components,pages,simulation})
e2e/                 Playwright specs (~20), config at root: playwright.config.ts
scripts/             coverage.sh, run-e2e.sh, check-licenses.sh, install.sh, ...
migrations           SYMLINK -> internal/db/migrations  (must stay identical)
docs/                architecture, deployment, API, security & audit reports
```

## Prerequisites

- **Go 1.26+**
- **Node.js 20+** (CI) / pnpm 9+ (CONTRIBUTING targets Node 22 / pnpm 9)
- **Docker + Docker Compose** (for the full stack)
- `sqlc` (only when changing SQL queries)

First-time setup:

```bash
make setup-hooks          # enable .githooks (sets core.hooksPath)
cd web && pnpm install && pnpm build && cd ..
```

## Build & Run

Prefer the Makefile targets over raw commands.

```bash
make build          # build web (pnpm) THEN go binaries (crm-server, crm-worker)
make run-local      # build web + run server locally on :8080
make up             # docker compose up -d --build (Caddy + server + worker + db)
make down           # docker compose down
make restart        # down + up
make logs           # live container logs
make clean          # remove bin/ and web/dist/
```

Cross-platform launcher builds: `make build-windows`, `build-darwin-arm64`, `build-darwin-amd64`, `build-linux-amd64`, `build-linux-arm64`, or `build-all-launcher`.

**Ordering matters:** the Go binary embeds `web/dist/*`. Always build the web frontend **before** `go build`, or the served SPA will be stale/empty. `make build` and `make run-local` handle this ordering for you.

## Testing

```bash
make test            # go test -v ./...
make test-e2e        # ./scripts/run-e2e.sh (builds server, runs Playwright)
make simulate        # 10-minute live visual simulation (headed)
make coverage        # coverage analysis, 50% total + 50% diff minimum
make check-coverage  # same gate as coverage
```

- **Unit/integration tests** live next to the code as `*_test.go`. CI runs them with the race detector (`go test -race`).
- **Coverage gate:** `scripts/coverage.sh --min 50.0 --diff 50.0` requires ≥ 50% total coverage **and** ≥ 50% coverage on newly added/changed lines. This is enforced by the **pre-push hook**. Write tests for new logic.
- **E2E** (`e2e/*.spec.ts`) runs against the built Go backend in demo mode + Vite frontend. `run-e2e.sh` builds `bin/crm-server` first.

## Lint, Format & Quality Gate

```bash
make check           # THE canonical local gate: setup-hooks + fmt + lint + check-licenses + test
make fmt             # gofmt -s -w .
make lint            # go vet ./...  +  (cd web && pnpm lint && pnpm typecheck)
make check-licenses  # 100% permissive-only license audit
```

- **Go:** `gofmt -s` (tabs), `go vet`, and `golangci-lint` (see `.golangci.yml`: errcheck, gosimple, govet nilness+shadow, ineffassign, staticcheck, unused, gosec, bodyclose, misspell, unconvert).
- **Web:** ESLint (`web/eslint.config.js`), Prettier, `tsc` typecheck.
- **Formatting rules** (`.editorconfig`): LF line endings, UTF-8, final newline, trim trailing whitespace. Go = tabs; web/shell = 2/4 spaces; Markdown may keep trailing spaces.

**Definition of done (local):** `make check` passes. For frontend changes, also ensure `cd web && pnpm format:check && pnpm lint && pnpm typecheck` are clean (CI enforces all of these).

## Git Workflow & Commit Conventions

- Branch from `main` using descriptive feature branches: `feat/...`, `fix/...`.
- Open PRs against `main` (use the PR template).
- **Conventional Commits** are enforced by the `commit-msg` hook. Valid types:
  `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`, `release`
  - Format: `<type>(<optional scope>)!<optional>!: <description>` (e.g. `feat(calendar): support recurring appointments`, `fix(auth): prevent unauthorized export access`)
  - `!` marks a breaking change. Keep the subject line ≤ ~80–100 chars.
  - Merge/fixup/squash/Revert commits are exempt.

## Git Hooks (`.githooks/`)

Enabled via `make setup-hooks` (sets `core.hooksPath=.githooks`). They run automatically and will block commits/pushes that violate repo invariants:

- **pre-commit:** secret/credential scan; verifies `migrations/` and `internal/db/migrations/` are identical; verifies SQLC-generated files are staged when `queries/*.sql` changed; `gofmt -s` on staged Go; `go vet`; web typecheck + ESLint on staged `web/` files; license audit when `go.mod`/`go.sum` changed.
- **commit-msg:** validates Conventional Commits format.
- **pre-push:** runs the coverage gate (50% total / 50% diff) + license scan.

If a hook fails, fix the root cause — do not bypass by editing the hook or using `--no-verify` unless a human explicitly asks.

## Critical Invariants (do not break)

1. **Migrations symlink:** `migrations` is a symlink to `internal/db/migrations`. The Go binary embeds `internal/db/migrations`. Both directories must contain identical files — pre-commit enforces this. Do not delete or replace the symlink with a real directory.
2. **SQLC workflow:** SQL queries live in `internal/db/queries/*.sql`. After editing any query file you **must** run `sqlc generate` and stage the regenerated `internal/db/*.sql.go`. Pre-commit blocks query changes without their generated output.
3. **New migrations:** add a new `NNNNN_name.sql` (zero-padded 5-digit prefix, sequential) to `internal/db/migrations/`. Never edit an already-applied migration — add a new one. Migrations must use LF endings (`.gitattributes`).
4. **Secrets:** never commit `.env`, `*.key`, `*.pem`, `backups/*.sql`, or `data/storage/**` keys. Only `.env.example` may be committed. Pre-commit blocks these and scans diffs for leaked keys.
5. **Embedded frontend:** `web/embed.go` uses `//go:embed dist/*`. Build web (`pnpm build`) before `go build`, or serve stale assets.
6. **Coverage:** keep new code covered (≥ 50% diff). Untested new logic will fail pre-push.
7. **License policy:** dependencies must be 100% permissive (MIT/Apache/BSD/etc.). **No GPL/AGPL/SSPL or other copyleft.** Run `make check-licenses` after touching `go.mod`/`go.sum`.

## Database & Migrations

- Engine: PostgreSQL 16 with `pgvector` and `pg_trgm`.
- Queries are written in `internal/db/queries/`, generated by `sqlc` (config: `sqlc.yaml`, output to `internal/db`, `pgx/v5`, emits JSON tags + interface).
- The migrator (`internal/db/migrator.go`) applies `internal/db/migrations/*` automatically at startup.
- Enum-like columns (`user_role`, `todo_status`, etc.) are mapped to Go `string` via `sqlc.yaml` overrides.
- **Do not introduce an ORM.** Keep queries explicit, typed, and compiled via sqlc.

## Configuration & Environment

Copy `.env.example` to `.env` (gitignored). Key variables (see `docs/CONFIGURATION_AND_ENV.md`):

- General: `DOMAIN`, `PORT`, `LOG_LEVEL`, `DEMO_MODE`
- Initial admin bootstrap: `INITIAL_ADMIN_EMAIL`, `INITIAL_ADMIN_PASSWORD`, `INITIAL_ADMIN_FIRST_NAME`, `INITIAL_ADMIN_LAST_NAME`
- Database: `DATABASE_URL` (or `DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_USER`/`DB_PASSWORD`)
- Auth: `JWT_SECRET_KEY_PATH` (fallback alias `JWT_PRIVATE_KEY_PATH`), `FORCE_SECURE_COOKIES`
- Storage: `STORAGE_PATH` (fallback alias `STORAGE_LOCAL_DIR`)
- Connectors: `CONNECTOR_API_TOKEN`
- AI: `AI_PROVIDER` (`ollama` default), `OLLAMA_BASE_URL`, `OLLAMA_MODEL`, `AI_API_KEY`, `AI_BASE_URL`

`docker-compose.yml` wires these into the `server`/`worker`/`db`/`caddy` services. `DB_PASSWORD` is required.

## Frontend Notes (`web/`)

- Vite + React 18 + TypeScript + Tailwind. Entry: `src/main.tsx`, router in `src/App.tsx`.
- API client: `src/api/client.ts`. Auth state: `src/context/AuthContext.tsx`. Realtime: `src/hooks/useSSE.ts`.
- Pages in `src/pages/` (Dashboard, Contacts, Companies, Deals, Inbox, AIAssistant, Automations, Calendar, MapView, Reports, Settings, Todos, Login). Feature components grouped under `src/components/<domain>/`.
- Use `@tanstack/react-query` for data fetching, `clsx` + `tailwind-merge` for class handling, `lucide-react` icons, `@dnd-kit` for the deals kanban.
- Prettier is the formatter (`.prettierrc`); run `pnpm format` / `pnpm format:check`.

## Security Guidelines

- Auth is Argon2id + Ed25519 JWT (HttpOnly cookies) + optional TOTP 2FA, with CSRF protection (`internal/server/csrf.go`, `internal/auth/csrf.go`) and rate limiting (`internal/auth/limiter.go`). Preserve these controls when touching auth or session code.
- Storage: sanitize all paths and block path traversal (`../`); files are content-addressed by SHA-256 (`internal/storage`).
- AI: PII must pass the guard (`internal/ai/guard.go`) before reaching any external LLM. Do not route raw customer PII to a provider without the guard.
- This repo has undergone multiple security & compliance review rounds (see `docs/SECURITY_REVIEW_*.md`, `docs/REPO_REVIEW_*.md`, `SECURITY.md`). Follow `SECURITY.md` for reporting issues; do not weaken existing hardening.
- Keep the RAM budget in mind — avoid heavy in-memory caches or unbounded allocations.

## Do / Don't

**Do**
- Follow existing per-domain structure (`internal/core/<domain>` + `internal/server/handlers/<domain>.go` + `*_test.go`).
- Add tests for new behavior; keep `make check` green.
- Use the Makefile targets and Conventional Commits.
- Keep SQL in `queries/` and regenerate with sqlc.

**Don't**
- Don't commit secrets, binaries, `bin/`, `coverage/`, `node_modules/`, or local data (`data/`, `backups/`).
- Don't add an ORM or bypass sqlc.
- Don't break the `migrations` symlink or the web-embed ordering.
- Don't skip or edit git hooks to force a bad commit through.
- Don't add copyleft-licensed dependencies.
