# Repo Review — Round 2 (2026-09-21)

Follow-up review of `openlocalcrm` at HEAD `380c7a7` (after the round-1 remediation of 41 findings, tracked in `docs/REPO_REVIEW_2026-09-21.md`). Scope: security, backend Go implementation, frontend, and docs/content accuracy. Read-only audit, no code changed. For processing: fix HIGH first, then MEDIUM, then LOW; each item has file:line + concrete fix.

## Critical / High

| # | Area | Finding | Fix |
|---|------|---------|-----|
| H1 | Security | `web/src/context/AuthContext.tsx:45,55,85,95` — raw JWT stored in `localStorage` even though backend already issues an httpOnly cookie for the same token (`internal/server/handlers/auth.go:97-105`). XSS = full session theft, bypassing httpOnly. | Drop localStorage token read/write; bootstrap session via `/api/v1/me` off the httpOnly cookie only. |
| H2 | Security | `.dockerignore` does not exclude `.env`, `.env.*`, `backups/`, `data/`, `*.sql`. `Dockerfile.server:15` `COPY . .` in `go-builder` stage can bake real secrets + a Postgres dump with password hashes into a build layer. | Mirror `.gitignore` rules into `.dockerignore`. |
| H3 | Backend/AI | `internal/ai/chat.go:70,417` `handleActionableIntent` — pure keyword/regex match on raw chat text directly calls `CreateCompany` with **no confirmation step**, unlike the automation engine's explicit `ApproveStep`/WAITING_APPROVAL flow. Any message like "lege Firma X als Kunden an" silently writes to the DB. | Route through a confirm/ActionCard step before writing. |
| H4 | Backend/AI | `internal/ai/chat.go:417` — `_, _ = s.querier.CreateCompany(...)` ignores the create error; reply unconditionally claims success (`chat.go:425`) even if the insert failed. | Check the error; only claim success on actual success. |
| H5 | Backend/AI | `internal/server/handlers/ai.go:146-171` `ParseBill` — always returns hardcoded fake data ("Familie Müller", fixed meter number), no file is even accepted. Mock posing as a real AI feature. | Implement real parsing or clearly mark endpoint as stub/disabled. |
| H6 | Backend | `internal/core/automation/service.go:353-388` `executeStep` — every action type (SET_TAG, CREATE_TASK, DRAFT_EMAIL, NOTIFY_USER, WEBHOOK) only logs and returns nil; nothing executes. Runs are still marked "COMPLETED". Entire `/api/v1/automations` feature is a no-op that lies about its own status. | Implement real action handlers or mark automations feature as not-yet-implemented in UI/docs. |
| H7 | Backend/API | `internal/core/appointment/service.go:97-99` + `internal/db/appointments.sql.go:140-146` `List`/`ListAppointments` — no LIMIT/OFFSET; `GET /api/v1/appointments` returns every row unbounded. | Add pagination, consistent with contacts/todos handlers. |
| H8 | Backend | `internal/server/handlers/backup.go:42-56` `Drill` — every query error is ignored (`userCount, _ :=` etc.) yet unconditionally reports `IntegrityCheck: "passed"`. A DB outage makes the backup-integrity endpoint falsely report success. | Check errors; fail the drill result on any query error. |
| H9 | Backend | `internal/server/handlers/backup.go:43-45,76-83` `Drill`/`Export` — hardcoded `Limit: 10000` for companies/contacts/deals/todos, no pagination or truncation warning. A "backup" silently drops data past 10k rows per table. | Paginate through full table or surface a truncation error. |
| H10 | Frontend | `web/src/pages/SettingsPage.tsx:465` — hardcoded fake connector "Bearer-Token" string shown in the UI on every install, never fetched from backend. | Fetch real per-tenant token or remove the field until wired up. |
| H11 | Frontend | `web/src/pages/SettingsPage.tsx:474-479` `handleTestWebhook` — pure client stub, no request ever sent, always shows a canned "Test-Lead erfolgreich eingespeist!" success message. | Call the real webhook test endpoint and reflect the actual response. |
| H12 | Frontend | `web/src/pages/SettingsPage.tsx:193-212,915-961` — 2FA setup discards `res.url` (the `otpauth://` URI); the displayed "QR code" is a static `lucide-react` icon, not a real QR code (no QR library in `web/package.json`). Users cannot actually scan-setup 2FA. Contradicts README's "QR-Code-Setup" claim (`README.md:117`). | Render `res.url` with a QR library (e.g. `qrcode.react`). |

## Medium

| # | Area | Finding | Fix |
|---|------|---------|-----|
| M1 | Security | `internal/server/handlers/auth.go` (multiple lines) — cookies set `Secure: r.TLS != nil`. In the documented deployment, TLS terminates at Caddy; the Go process only ever sees plaintext, so `r.TLS` is always nil and `Secure` is always false in production. | Derive "secure" from a trusted `X-Forwarded-Proto` (or explicit `FORCE_SECURE_COOKIES` env flag), not `r.TLS`. |
| M2 | Security | `internal/auth/service.go:46-67` — access-token revocation (`sync.Map`) is process-local and in-memory only; a restart or multi-replica deployment un-revokes a "logged out" token for up to 24h. | Persist revocations (DB/shared cache) or shorten access-token TTL. |
| M3 | Security | `internal/auth/limiter.go:229-236` + `router.go:87` — trusts `X-Forwarded-For`/`X-Real-IP` with no trusted-proxy allowlist; `docker-compose.demo.yml:24-25` exposes the Go server directly on `8080:8080`, letting an attacker spoof the header to bypass login lockout. | Don't expose the app port directly in compose; only trust forwarded headers from the known proxy. |
| M4 | Backend/AI | `internal/server/handlers/ai.go:311-356` `CreateResearchJob` — persists job as `Status: "COMPLETED"` immediately with fabricated boilerplate result; no real research runs (separate from the real SSRF-guarded `ResearchCompany`/`internal/ai/research.go`). | Implement real async research or remove/mark stub endpoint. |
| M5 | Backend/AI | `internal/ai/gateway.go:140-141,181-183` — `callOpenAI`/`callOllama` silently fall back to canned `simulateGemmaResponse` text on any HTTP failure/non-200, indistinguishable from a real answer except by a heuristic string match. | Surface a distinct error/degraded-mode signal instead of silent fallback text. |
| M6 | Backend | `internal/queue/jobs.go:23-38,53-67` — `EmailSyncWorker`/`EmailTriageWorker` are stubs that only log and return nil despite being registered as real jobs. | Implement or remove from the job registry until implemented. |
| M7 | Backend/API | `internal/core/deal/service.go:139-141` + `internal/db/deals.sql.go:165-172` `ListByStage` — no limit clause, unbounded, unlike the clamped default `List`. | Add pagination. |
| M8 | Backend/API | `internal/server/handlers/deals.go:64-72` — default `List` hardcodes `limit=100, offset=0`, ignores `?limit=&offset=` query params entirely (contacts/companies/todos read them). | Read pagination params consistently across handlers. |
| M9 | Backend/API | `internal/server/handlers/ai.go:247-254` `DeleteKB` — if `uuid.Parse(id)` fails, DB delete is silently skipped but handler still returns `{"success": true}`. | Return 400 on parse failure instead of a false success. |
| M10 | Backend | `internal/server/router.go:202-203` — `auth.NewRateLimiter` starts a `cleanupLoop` goroutine that is never `Stop()`'d anywhere; `NewAuthHandler` creates a second, independently-leaked limiter. Every router/test construction leaks a goroutine. | Call `Stop()` on shutdown; reuse a single limiter instance instead of constructing a second one. |
| M11 | Frontend | `web/src/context/AuthContext.tsx` / `web/src/api/client.ts` — JWT in localStorage *and* a separate httpOnly-cookie CSRF/session mechanism coexist (two auth transports mixed); ties into H1/M1. | Consolidate on the httpOnly cookie path. |
| M12 | Frontend | `web/src/pages/SettingsPage.tsx:361-388` `handleImportSettings` — only a truthy top-level-key check before POSTing raw parsed JSON to `/api/v1/settings/import`; no schema/type validation of nested fields. | Validate shape/types client-side, or don't imply client-side safety. |
| M13 | Docs | `web/src/pages/SettingsPage.tsx:1398` (in-app "System-Info" tab) hardcodes "Go 1.23 + Caddy + Chi", contradicting both `go.mod` (`go 1.26.0`) and the README badge. Live UI mismatch, not just a stale badge. | Derive from a build-time constant or `/api/v1/health` payload. |
| M14 | Docs | `docs/CONFIGURATION_AND_ENV.md` vs `.env.example` vs code — `AI_API_KEY`/`AI_BASE_URL` documented and read in code but missing from `.env.example`; `INITIAL_ADMIN_FIRST_NAME`/`INITIAL_ADMIN_LAST_NAME` read directly in `cmd/server/main.go:94,98` but undocumented anywhere and absent from `.env.example`. | Reconcile all three sources so nothing is orphaned. |

## Low

- **Security** — `internal/ai/gateway.go:19-22,87-94`: `ProviderAnthropic`/`ProviderGemini` declared but unhandled in `Generate`'s switch, silently falls back to Ollama; not exploitable today but risks bypassing the PII-guard wrapper if wired up later without extending the switch. Fix: exhaustive switch or fail closed.
- **Security** — No HTTP security headers (CSP/HSTS/X-Frame-Options) at the Go app layer, only in `Caddyfile:9-23`; combined with M3's direct port exposure in demo compose, a direct hit gets none of them. Fix: minimal header set at app layer as defense in depth, or stop exposing the port directly.
- **Backend** — `internal/core/automation/service.go:64-165` `ListDefaultWorkflows` falls back to 3 hardcoded demo workflows when DB is empty; combined with H6, a fresh install's automation UI shows workflows that can never do anything real.
- **Backend** — `internal/connectors/connector.go:77-85` `IngestLead`: bad numeric silently becomes zero (`_ = val.Scan(...)`), deal-creation failure silently dropped (`_, _ = e.dealSvc.Create(...)`) — webhook lead-intake can report success while losing the deal.
- **Backend** — `internal/server/handlers/deals.go:170` `_ = val.Scan(req.Value)`: malformed numeric silently ignored instead of returning 400.
- **Backend** — Widespread `_ = json.NewDecoder(...).Decode(&req)` / `_ = json.NewEncoder(w).Encode(...)` ignoring errors across handlers (ai.go, todos.go, deals.go, contacts.go, notes.go, auth.go) — low risk but zero operational visibility into malformed requests or broken client connections.
- **Backend** — `cmd/server/main.go:209` `_ = srv.Shutdown(shutdownCtx)` swallows shutdown timeout/error — operators get no signal that connections were force-closed.
- **Frontend** — 61 occurrences of `: any` across `web/src` (DealsPage, CalendarPage, ReportsPage, ActivityLogDrawer, CommandPalette, etc.) hiding potential shape mismatches with the Go backend. Fix: shared DTO types, ideally generated from Go structs/OpenAPI.
- **Frontend** — No `AbortController`/unmount-guard anywhere in `web/src`; `AuthContext.tsx`/`SettingsPage.tsx` `useEffect` fetches can `setState` after unmount on slow requests.
- **Docs** — RAM budget "< 420 MB" (`README.md:10,134-145`) has no benchmark/CI check anywhere in the repo; unverified marketing number stated as a hard guarantee.
- **Docs** — Playwright badge "27 passed (100%)" is currently accurate (27 `test(` calls across `e2e/*.spec.ts`) but hand-maintained with no CI regeneration — will silently rot.
- **Docs** — README/CONTRIBUTING/SECURITY/CODE_OF_CONDUCT are entirely German, no English version or i18n scaffolding, despite MIT/open-source framing for external contributors.
- **Docs** — Legacy "mavalio" branding still present and user-facing in `docs/GETTING_STARTED.md:43-44`, `docs/DEPLOYMENT.md:122` ("Legacy-Alias" admin/vertrieb emails); also in `cmd/setup-launcher/dir.go:42-46` (hardcoded `C:\mavalio` paths), `internal/launcher/engine.go`, `internal/launcher/config.go:162`, `internal/db/demo/querier.go`, and shipped release binaries `mavalio-setup.exe`/`mavalio-setup-debug.exe` (`Makefile:28,33`, `.github/workflows/docker-publish.yml:125,128`). Already flagged in round 1 (`docs/REPO_REVIEW_2026-09-21.md:34,51-52`) but unresolved.
- **Docs** — Quickstart ("Schnellstart & Deployment") starts at README.md:146 of 310 total lines — 42% marketing/architecture content precedes runnable install instructions.

## Verified clean (no action needed)

- Ed25519 JWT with explicit alg pinning (`internal/auth/jwt.go:44-50`), Argon2id password hashing with constant-time compare (`internal/auth/password.go`), CSRF double-submit constant-time compare (`internal/auth/csrf.go:96`).
- No raw/unparameterized SQL string-building found in `internal/db` or `internal/server` (sqlc/pgx only).
- Path-traversal-safe storage layer (`internal/storage/local.go:45-51` `sanitizeRelPath`), CAS dedup, magic-byte MIME validation.
- SSRF-hardened AI research HTTP client with private/reserved-IP blocking, resolved-and-dialed atomically (`internal/ai/research.go:36-102`).
- Constant-time connector webhook token check (`internal/connectors/connector.go:49`); last-admin-protection on role/status changes (`internal/server/handlers/users.go:168-181,233-246`).
- No `dangerouslySetInnerHTML`/`.innerHTML` anywhere in `web/src`; no hardcoded `localhost`/absolute API URLs; top-level `ErrorBoundary` correctly mounted (`web/src/main.tsx:21`).
- `internal/sse/hub.go`: mutex-protected client map, non-blocking broadcast with buffered channel + drop, context-cancellation-aware, no leak found.
- `internal/db/migrator.go`: per-migration transaction with explicit rollback on failure — solid.
- PII-scrub guard (`internal/ai/guard.go`) genuinely implemented and wired into the AI gateway, not just a doc claim.
- README §1–§22 feature-claim spot check (audit-log, EU AI Act observability, DSGVO PII-masking) — all three substantiated by real code.

## Cross-cutting theme

The AI copilot and automation subsystems (H3–H6, H9, M4–M6, and the "Verified clean" PII guard) are the highest-risk area: several endpoints advertised as AI-driven features (`ParseBill`, `CreateResearchJob`, automation `executeStep`) return fabricated or no-op results while reporting success, and the one endpoint that *does* write real data from free text (`handleActionableIntent`) does so without the confirmation gate the rest of the system uses. Recommend treating "does this AI feature actually do what its response claims" as a single remediation pass rather than fixing each endpoint in isolation.
