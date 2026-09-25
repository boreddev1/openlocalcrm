# Sub-Project 7 (SP7): Backup, Invite & Settings Design Spec

**Status:** Approved
**Target Branch:** `fix/sp7-backup-settings`
**Baseline:** `main` at `e0dc7747b22125997980a052f5308892eb048f4e`
**Parent Design:** `docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md` §11
**Parent Plan:** `docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md` Phase 7
**Findings Addressed:** B1, B2, B3, B4, B5, B6

---

## 1. Purpose and Boundaries

SP7 replaces the remaining fake-success backup, invite, 2FA, template, and webhook-test paths with real persistence and honest failures. The approved parent design is binding; this document records the concrete SP7 architecture, data flow, failure behavior, and verification contract.

SP7 does not modify or reimplement SP1 through SP6. It does not change the 1024-dimensional embedding contract, the `00010_foreign_key_indexes.sql` file, or any previously applied migration. The next migration is `00018`; the following SP7 migration is `00019`.

SP7 operates against the currently migrated application schema. Historical migration replay from an empty database is outside this sub-project.

## 2. Data Migrations

### 2.1 `00018_user_invites.sql`

`user_invites` stores one-time invite state:

- `id UUID PRIMARY KEY`;
- `user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE`;
- `token_hash VARCHAR(64) UNIQUE NOT NULL`, containing only the lowercase SHA-256 hex digest;
- `created_by UUID REFERENCES users(id) ON DELETE SET NULL`;
- `expires_at TIMESTAMPTZ NOT NULL`;
- `consumed_at TIMESTAMPTZ NULL`;
- `revoked_at TIMESTAMPTZ NULL`;
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`.

Active invites are indexed by user and expiry. Raw invite tokens are never persisted, logged, or returned by a list endpoint.

### 2.2 `00019_email_templates.sql`

`email_templates` stores durable, editable template definitions:

- identity and display name;
- subject and body;
- category and locale metadata;
- created/updated timestamps.

The table has a unique stable key used by upserts and CRUD operations. The template generator calls the existing real AI gateway; it does not create rows or return canned text without work.

### 2.3 Generated Bindings

All custom SQL is added through `internal/db/queries/*.sql` and regenerated with `sqlc generate`. The commits that add the migrations and queries include the generated `internal/db/*.sql.go`, `querier.go`, and `models.go` files.

## 3. B1 — Full Streaming Backup Export

### 3.1 Service Boundaries

A new focused package `internal/core/backup` owns backup serialization, restore validation, ordering, and audit orchestration. HTTP handlers own request authentication, response headers, upload limits, and streaming lifecycle only.

The service receives `db.Querier` for static queries. The PostgreSQL pool is used only for streaming coordination and scratch-schema lifecycle. The in-memory demo querier cannot perform a real scratch-schema restore or drill.

### 3.2 Table Inventory and Order

The current application inventory contains 22 tables. SP7 backup and restore also include the two tables added by `00018` and `00019` once they exist. The restore order is the FK-safe insertion order:

1. `users`
2. `companies`
3. `workflows`
4. `contacts`
5. `deals`
6. `email_accounts`
7. `todos`
8. `refresh_tokens`
9. `audit_logs`
10. `notifications`
11. `email_messages`
12. `email_attachments`
13. `appointments`
14. `call_activities`
15. `company_research`
16. `workflow_runs`
17. `workflow_steps`
18. `notes`
19. `intake_forms`
20. `user_invites`
21. `email_templates`
22. `knowledge_base_articles`
23. `ai_research_jobs`
24. `ai_audit_logs`

`schema_migrations` is infrastructure metadata and is not part of an application backup. Reverse order is used for replace-mode truncation.

### 3.3 Handwritten Row Streamers

`internal/db/queries/backup.sql` is the canonical SQL source for the 22 pre-SP7 table selects and their restore upserts. `user_invites` and `email_templates` have their own select and upsert queries in `invites.sql` and `email_templates.sql`. All 24 row queries return `row_to_json(...)` values ordered by primary key.

sqlc `:many` methods materialize `[]T`; they must not be used by the export path. A handwritten `internal/db/backup_stream.go` in `package db` implements all 24 streaming methods because generated methods cannot expose the unexported `Queries.db` handle for caller-driven `rows.Next()` iteration. The handwritten methods reuse the generated, package-private query constants, call `q.db.Query`, and emit one scanned JSON row at a time. They never append rows to a slice.

Existing helpers such as `ListCompanies`, `ListContacts`, `ListDeals`, and `ListTodos` are prohibited in the export path because they accumulate `[]T` in memory.

### 3.4 Wire Format

The response is a versioned JSON envelope:

```json
{
  "format": "openlocalcrm-backup",
  "version": 1,
  "exported_at": "RFC3339 timestamp",
  "tables": {
    "users": [],
    "companies": []
  }
}
```

The handler writes the metadata and table boundaries, then passes each scanned row directly to `json.Encoder`. It does not build an `ExportPayload` or another whole-dump slice. JSONB and other `[]byte` fields remain JSON values rather than base64 strings because rows originate from `row_to_json`.

Round-trip typing is explicit in the static restore queries. `knowledge_base_articles.embedding` is cast back to `vector(1024)`, while `knowledge_base_articles.tags` and `company_research.industry_keywords` are cast from JSON arrays back to `TEXT[]`. Row JSON alone is not passed directly into these typed columns, and the vector dimension is never reduced to the obsolete 768 contract.

### 3.5 Failure and Audit Behavior

Export is admin-only. A failure before the response starts returns a normal 4xx/5xx JSON error. A failure after streaming starts aborts the HTTP response rather than returning a valid success document; clients must reject truncated JSON. No handler reports success after partial work.

Export creates audit events with the authenticated actor, real client IP, and real user agent. The event distinguishes started, completed, and failed exports without embedding the exported payload or sensitive row values.

## 4. B2 — Transactional Restore and Real Drill

### 4.1 Restore Endpoint

`POST /api/v1/backup/restore` is admin-only and accepts the versioned backup envelope through `http.MaxBytesReader`.

The request validates:

- format and version;
- table allowlist and dependency order;
- row object shape and table column names;
- primary keys and JSON value types;
- request size before transaction work.

Merge is the default mode. Replace mode requires an explicit request flag, truncates tables in reverse FK order, and performs the complete import in one transaction. Any insert, validation, count, or commit error rolls back the entire restore. There is no partial success response.

Static per-table upsert statements come from `backup.sql` and are regenerated with sqlc. Restore runs them through `db.Queries.WithTx`; identifiers are never assembled from request strings.

Restore success and failure are audit-logged with actor, IP, and user agent. A completion event is inserted only after the restore transaction commits; it is never inserted inside a replace transaction because restoring `audit_logs` would delete that event. Failure is logged after rollback. The HTTP response reports success only when the post-commit audit insert also succeeds.

### 4.2 Scratch-Schema Drill

`POST /api/v1/backup/drill` has no request body. When a PostgreSQL pool is available, it performs a live export of the current `public` tables into the scratch schema and verifies that imported result:

1. Create a unique scratch schema.
2. Clone each application table with `CREATE TABLE scratch.<table> (LIKE public.<table> INCLUDING ALL)`.
3. Inspect the foreign-key constraints copied by `INCLUDING ALL`.
4. Drop every copied constraint that still references `public` before adding any scratch constraint. Leaving it in place would validate imported rows against live data instead of scratch data.
5. Add equivalent foreign keys that reference `scratch.<parent table>`.
6. Stream every current live row from `public` through the same table descriptors into the scratch schema; there is no upload or client-supplied backup source. The drill uses two distinct `db.Queries` handles: the source handle runs on a connection whose effective schema is `public`, while the target handle runs inside the scratch transaction after `SET LOCAL search_path` resolves writes to `scratch`. One shared `search_path` is forbidden because it would make the unqualified streamer SQL read the empty scratch tables and produce a false `0 = 0` pass.
7. Compare per-table row counts.
8. Enforce constraints immediately and run the FK verification.
9. Drop the scratch schema in a guaranteed cleanup path, including failure paths.

The drill reports per-table counts and verification status. It does not call only `COUNT(*)` reads and does not return `passed` when no import occurred.

In demo mode there is no PostgreSQL pool or transactional scratch schema. The drill and restore return HTTP 501 with an honest unavailable message; they never simulate success.

## 5. B3 — Token-Based Invite Flow

### 5.1 Issuance

Invite issuance creates a 32-byte cryptographically random token, returns it once as a copyable link, and stores only its SHA-256 hash. The invite expires after 72 hours. The new user remains `INVITED` and cannot log in before acceptance.

The admin UI displays the link and offers copy/download actions. It does not claim that an email was sent.

### 5.2 Validation and Acceptance

`GET /api/v1/auth/accept-invite?token=...` is read-only. It returns safe invite metadata for a valid, unconsumed, unrevoked, unexpired token and an honest error for an invalid token.

`POST /api/v1/auth/accept-invite` accepts the token and new password. In one transaction it:

1. locks the invite row with `SELECT ... FOR UPDATE`;
2. verifies it is active and unexpired;
3. hashes the new password with Argon2id;
4. sets the user password and status to `ACTIVE`;
5. sets `consumed_at = NOW()`;
6. commits.

A consumed, revoked, expired, or unknown token is rejected and never reactivates an account. A second acceptance attempt cannot reuse the token. The acceptance endpoint is public and CSRF-exempt like login, with strict payload and rate limits.

Invite issuance, rejection, and acceptance are audit-logged without storing raw tokens.

## 6. B4 — Real 2FA Disable

The settings UI hydrates `totp_enabled` from the authenticated session rather than local default state. Disabling 2FA requires password confirmation and calls the existing `POST /api/v1/auth/totp/disable` endpoint.

The UI updates its state only from a successful server response. A failed request leaves 2FA enabled and displays the real error.

## 7. B5 — Real Email Templates

The settings UI provides DB-backed template CRUD backed by `email_templates` and admin-only endpoints. Create, update, delete, list, and read operations persist real rows.

Template generation calls the existing real AI chat/gateway path and returns the model result. Provider failure returns an error; there is no delayed canned response. Saving a generated template requires the normal persisted create/update operation, not a success toast without a write.

## 8. B6 — Honest Webhook Test

The settings UI contains an empty masked connector-token field. The value is held only in component state for the current test, is never persisted, and has no production or demo default value.

The browser sends the real contract directly:

- `Authorization: Bearer <entered token>`;
- a schema-valid `LeadIntakePayload` with real `first_name`, `last_name`, optional contact/deal fields, and string `deal_value`;
- honest display of the returned `contact_id`, deal result, or 4xx/5xx error.

There is no `X-Connector-Token` fallback and no fabricated success message.

## 9. Frontend Data Flow

- Backup download uses authenticated `fetch` and streams `response.body` to a file instead of `apiFetch` plus `JSON.stringify`.
- Invite acceptance is a public route that loads by token and submits the password with POST.
- Settings mutations display success only after the corresponding request and invalidation succeed.
- Query keys are invalidated for invites, templates, users, deals, and reports only after real persistence.

## 10. Tests and Quality Gates

### 10.1 Required Tests

- `backup_test.go`:
  - no export path calls a `List*` helper;
  - each table row is emitted once and in FK-safe order;
  - JSONB fields are not base64 encoded;
  - `vector(1024)` and `TEXT[]` values survive the export/restore round trip with explicit casts;
  - the drill reads live `public` data through a source handle and writes through a separate scratch target handle, never one shared `search_path`;
  - a seeded non-empty source produces matching non-zero scratch counts rather than a false `0 = 0` pass;
  - the drill accepts no body or uploaded backup;
  - replace-mode completion audit is written only after commit and survives the import;
  - merge upserts are idempotent;
  - replace truncation and inserts roll back as one transaction;
  - unknown tables/columns, invalid JSON, and oversized bodies fail;
  - export and restore create audit events with request metadata;
  - demo restore/drill return 501;
  - scratch tables contain no remaining `public` foreign keys before scratch constraints are added;
  - the scratch schema is removed on success and failure.
- Invite tests:
  - raw token is not stored;
  - `GET` does not mutate;
  - `POST` consumes the token and activates the account;
  - reuse, expiry, revocation, and concurrent acceptance fail safely.
- TOTP tests:
  - disable requires password confirmation and the real endpoint.
- Template tests:
  - CRUD persistence and real AI failure behavior.
- Webhook tests:
  - Bearer header, valid payload, no default token, honest error rendering.
- Playwright:
  - replace the fake invite toast with the copyable link;
  - accept the invite in a clean context and log in;
  - verify the disabled 2FA state after a real request;
  - verify webhook failure and success responses.

### 10.2 Commands

After SP7 implementation:

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck && pnpm build
go test -race ./...
./scripts/coverage.sh --min 50.0 --diff 50.0
make check
test -L migrations && ls -l migrations
```

Coverage must remain at least 50% overall and at least 50% for changed lines. The migrations symlink must continue to point to `internal/db/migrations`.

## 11. Commit and Integration Contract

SP7 uses three Conventional Commits:

1. `feat(db): add invite tokens and email template storage`
2. `feat(backup): implement full streaming backup, restore, and real restore drill (B1, B2)`
3. `feat(settings): add token invites, 2FA wiring, templates, and webhook test (B3-B6)`

After verification, `fix/sp7-backup-settings` is merged locally into `main`. SP8a, SP8b, SP8c, and SP9 each receive their own approved design, plan, implementation branch, review, and local merge in that order. Nothing is pushed.
