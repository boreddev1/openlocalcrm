# SP7 Backup, Invite & Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace SP7 fake-success backup, invite, 2FA, template, and webhook-test paths with bounded-memory streaming, real persistence, and honest failure responses.

**Architecture:** Add `00018_user_invites.sql` and `00019_email_templates.sql`, then keep all application SQL in sqlc-managed query files. A handwritten package-`db` row source streams `row_to_json` results without `[]T`; `internal/core/backup` owns the table registry, streaming envelope, transactional restore, and scratch-schema drill. The frontend uses streamed `fetch`, public invite validation/acceptance, real TOTP disable, DB-backed templates, and a browser-held Bearer token for the webhook test.

**Tech Stack:** Go 1.26, Chi, pgx/v5, pgxpool, PostgreSQL 16 + pgvector, sqlc, React 18, TypeScript, Vite, TanStack Query, Playwright.

## Global Constraints

- Approved spec: `docs/superpowers/specs/2026-09-26-sp7-backup-invite-settings-design.md` at `c258fd0`.
- Work on `fix/sp7-backup-settings`, created from the updated `main` after the plan commit.
- Do not modify SP1–SP6 behavior, `00010_foreign_key_indexes.sql`, or any applied migration.
- New migrations are exactly `00018_user_invites.sql` and `00019_email_templates.sql`; never edit an older migration.
- Keep `migrations -> internal/db/migrations` intact.
- SQL lives in `internal/db/queries/*.sql`; run `/Users/ewenwitte/go/bin/sqlc generate` and commit generated files.
- No ORM and no whole-dump `[]T` accumulator. Export code must not call generated `List*` or generated `:many` row methods.
- `knowledge_base_articles.embedding` remains `vector(1024)`; `tags` and `industry_keywords` round-trip as `TEXT[]` with explicit casts.
- The drill uses separate source (`public`) and target (`scratch`) `db.Queries` handles. Never share one `search_path` between reads and writes.
- Restore defaults to merge. Replace is explicit, transactional, and reverse-ordered.
- Restore success is reported only after commit and post-commit audit. Rollback and post-commit audit failure use distinct error codes.
- Demo restore/drill return HTTP 501. Demo export is allowed only through a row-streaming demo source, never `List*` helpers.
- Invite tokens are 32 random bytes, expire after 72 hours, and only SHA-256 hashes are persisted.
- `GET /api/v1/auth/accept-invite` is read-only; `POST` consumes the invite and activates the user.
- The connector token field starts empty, is not persisted, and has no production/demo default.
- Zero fake success: no success toast, 2xx, or report status without the corresponding real work.
- Three commits only, with the exact messages in Task 1, Task 3, and Task 6.
- Dependencies stay permissively licensed; run `make check-licenses` after dependency changes.
- Coverage must remain at least 50% total and at least 50% for changed lines.
- Do not push.

---

## File Structure

**Create**

- `internal/db/migrations/00018_user_invites.sql` — one-time invite state.
- `internal/db/migrations/00019_email_templates.sql` — durable template CRUD storage.
- `internal/db/queries/invites.sql` — invite issuance, validation, lock/consume, revoke, export, restore.
- `internal/db/queries/email_templates.sql` — template CRUD, export, restore.
- `internal/db/queries/backup.sql` — 22 existing table row selectors and static restore statements.
- `internal/db/backup_stream.go` — handwritten package-`db` callers of generated query constants.
- `internal/db/demo/backup_stream.go` — page-local, map-iterating demo row source.
- `internal/core/backup/service.go` — export/restore/drill orchestration and audit boundaries.
- `internal/core/backup/tables.go` — 24 table names, order, row source, upsert, and validation registry.
- `internal/core/backup/scratch.go` — scratch clone, FK rewrite, source/target handles, cleanup.
- `internal/core/invite/service.go` — token generation, hashing, validation, transactional acceptance.
- `internal/core/emailtemplate/service.go` — template CRUD and real AI generation.
- `internal/server/handlers/invites.go` — public validate/accept endpoints.
- `internal/server/handlers/email_templates.go` — admin template CRUD/generation endpoints.
- `web/src/pages/AcceptInvitePage.tsx` — public token validation and password form.

**Modify**

- `internal/db/demo/querier.go` and generated `internal/db/querier.go` — new interface methods and demo storage.
- `internal/server/handlers/backup.go` — remove `ExportPayload`; stream, restore, and honest drill responses.
- `internal/server/handlers/users.go` — invite issuance returns the one-time link.
- `internal/server/handlers/auth.go` — `/me` exposes `totp_enabled`; no SP8a token-body changes here.
- `internal/server/router.go` — pool injection and new routes.
- `cmd/server/main.go` — pass the real pool; leave demo pool nil.
- `web/src/pages/SettingsPage.tsx` — invite link, streamed backup upload/download, TOTP disable, templates, Bearer webhook.
- `web/src/App.tsx`, `web/src/api/client.ts`, `web/src/context/AuthContext.tsx` — public invite route, streamed helpers, TOTP session field.
- `e2e/auth.spec.ts`, `e2e/connectors.spec.ts`, `e2e/live-simulation.spec.ts` — honest real-request assertions.

---

### Task 1: Invite and Template Persistence

**Files:**
- Create: `internal/db/migrations/00018_user_invites.sql`
- Create: `internal/db/migrations/00019_email_templates.sql`
- Create: `internal/db/queries/invites.sql`
- Create: `internal/db/queries/email_templates.sql`
- Modify: `internal/db/demo/querier.go`
- Regenerate: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

**Interfaces:**
- Produces: `db.UserInvite`, `db.EmailTemplate`, `CreateUserInvite`, `GetUserInviteByTokenHash`, `RevokeUserInvite`, `ListUserInvites`, and the template CRUD methods used by Tasks 4–5.
- Invites are ordered after `intake_forms`; templates are ordered after invites.

- [ ] **Step 1: Write failing storage tests**

Add `internal/db/invites_templates_test.go`:

```go
func TestUserInviteAndTemplateContracts(t *testing.T) {
    t.Run("invite token hash is unique", func(t *testing.T) {
        migrations, err := GetEmbeddedMigrations()
        require.NoError(t, err)
        require.Contains(t, migrations, "00018_user_invites.sql")
        require.Contains(t, migrations, "00019_email_templates.sql")
    })

    t.Run("demo invite state is persisted", func(t *testing.T) {
        q := NewInMemoryQuerier()
        ctx := context.Background()
        userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
        expires := time.Now().UTC().Add(72 * time.Hour)
        invite, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
            UserID: userID, TokenHash: strings.Repeat("a", 64),
            ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
        })
        require.NoError(t, err)
        got, err := q.GetUserInviteByTokenHash(ctx, strings.Repeat("a", 64))
        require.NoError(t, err)
        require.Equal(t, invite.ID, got.ID)
    })
}
```

Use the repository's existing assertion style if `testify/require` is not already present in this package; do not add a dependency solely for this test.

- [ ] **Step 2: Run the test and verify red**

Run:

```bash
go test ./internal/db -run TestUserInviteAndTemplateContracts -count=1
```

Expected: FAIL because the models and queries do not exist.

- [ ] **Step 3: Add `00018_user_invites.sql`**

```sql
-- migrations/00018_user_invites.sql
-- +goose Up
CREATE TABLE user_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_invites_user_id ON user_invites(user_id);
CREATE INDEX idx_user_invites_active ON user_invites(expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
-- +goose Down
DROP TABLE IF EXISTS user_invites;
```

- [ ] **Step 4: Add `00019_email_templates.sql`**

```sql
-- migrations/00019_email_templates.sql
-- +goose Up
CREATE TABLE email_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    subject VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT '',
    locale VARCHAR(16) NOT NULL DEFAULT 'de-DE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_email_templates_category ON email_templates(category);
-- +goose Down
DROP TABLE IF EXISTS email_templates;
```

- [ ] **Step 5: Add invite queries**

Create `internal/db/queries/invites.sql` with:

```sql
-- internal/db/queries/invites.sql
-- name: CreateUserInvite :one
INSERT INTO user_invites (user_id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserInviteByTokenHash :one
SELECT * FROM user_invites
WHERE token_hash = $1
LIMIT 1;

-- name: RevokeUserInvite :exec
UPDATE user_invites
SET revoked_at = NOW()
WHERE id = $1 AND consumed_at IS NULL;

-- name: ListUserInvites :many
SELECT * FROM user_invites ORDER BY created_at DESC;

-- name: ExportUserInvitesRows :many
SELECT row_to_json(t) FROM user_invites t ORDER BY t.id;

-- name: UpsertUserInvitesRow :exec
INSERT INTO user_invites (id, user_id, token_hash, created_by, expires_at, consumed_at, revoked_at, created_at)
SELECT (r->>'id')::uuid, (r->>'user_id')::uuid, r->>'token_hash',
       (r->>'created_by')::uuid, (r->>'expires_at')::timestamptz,
       (r->>'consumed_at')::timestamptz, (r->>'revoked_at')::timestamptz,
       (r->>'created_at')::timestamptz
FROM jsonb_to_record($1::jsonb) AS r(
    id text, user_id text, token_hash text, created_by text,
    expires_at text, consumed_at text, revoked_at text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id, token_hash = EXCLUDED.token_hash,
    created_by = EXCLUDED.created_by, expires_at = EXCLUDED.expires_at,
    consumed_at = EXCLUDED.consumed_at, revoked_at = EXCLUDED.revoked_at;
```

Use the same explicit `jsonb_to_record` shape in `email_templates.sql` for `id`, `key`, `name`, `subject`, `body`, `category`, `locale`, and timestamps, with `ON CONFLICT (key) DO UPDATE`.

- [ ] **Step 6: Add template queries**

Create `internal/db/queries/email_templates.sql` with `CreateEmailTemplate :one`, `GetEmailTemplateByKey :one`, `ListEmailTemplates :many`, `UpdateEmailTemplate :one`, `DeleteEmailTemplate :exec`, `ExportEmailTemplatesRows :many`, and `UpsertEmailTemplatesRow :exec`. Use `updated_at = NOW()` in the update query and `ORDER BY t.id` in the export selector.

- [ ] **Step 7: Implement demo storage**

Add `userInvites` and `emailTemplates` maps to `InMemoryQuerier`, initialize them in `NewInMemoryQuerier`, and implement every newly generated method. `RevokeUserInvite` must preserve the SQL predicate that consumed rows are not mutated.

- [ ] **Step 8: Regenerate sqlc and verify green**

Run:

```bash
/Users/ewenwitte/go/bin/sqlc generate
gofmt -s -w internal/db
go test ./internal/db/... -count=1
```

Expected: PASS and a clean diff containing generated files plus the two new migrations.

- [ ] **Step 9: Commit Task 1**

```bash
git add internal/db/migrations/00018_user_invites.sql \
  internal/db/migrations/00019_email_templates.sql \
  internal/db/queries/invites.sql internal/db/queries/email_templates.sql \
  internal/db/*.sql.go internal/db/models.go internal/db/querier.go internal/db/demo
git commit -m "feat(db): add invite tokens and email template storage"
```

---

### Task 2: Bounded-Memory Streaming Export

**Files:**
- Create: `internal/db/queries/backup.sql`
- Create: `internal/db/backup_stream.go`
- Create: `internal/db/demo/backup_stream.go`
- Create: `internal/core/backup/service.go`
- Create: `internal/core/backup/tables.go`
- Modify: `internal/server/handlers/backup.go`
- Modify: `internal/server/router.go`
- Modify: `cmd/server/main.go`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/pages/SettingsPage.tsx`
- Test: `internal/core/backup/service_test.go`, `internal/server/handlers/backup_test.go`

**Interfaces:**
- Consumes: Task 1 generated `UserInvite` and `EmailTemplate` queries.
- Produces:

```go
type RowSink func(context.Context, json.RawMessage) error

type RowSource interface {
    ScanBackupUsers(context.Context, RowSink) error
    ScanBackupCompanies(context.Context, RowSink) error
    ScanBackupWorkflows(context.Context, RowSink) error
    ScanBackupContacts(context.Context, RowSink) error
    ScanBackupDeals(context.Context, RowSink) error
    ScanBackupEmailAccounts(context.Context, RowSink) error
    ScanBackupTodos(context.Context, RowSink) error
    ScanBackupRefreshTokens(context.Context, RowSink) error
    ScanBackupAuditLogs(context.Context, RowSink) error
    ScanBackupNotifications(context.Context, RowSink) error
    ScanBackupEmailMessages(context.Context, RowSink) error
    ScanBackupEmailAttachments(context.Context, RowSink) error
    ScanBackupAppointments(context.Context, RowSink) error
    ScanBackupCallActivities(context.Context, RowSink) error
    ScanBackupCompanyResearch(context.Context, RowSink) error
    ScanBackupWorkflowRuns(context.Context, RowSink) error
    ScanBackupWorkflowSteps(context.Context, RowSink) error
    ScanBackupNotes(context.Context, RowSink) error
    ScanBackupIntakeForms(context.Context, RowSink) error
    ScanBackupUserInvites(context.Context, RowSink) error
    ScanBackupEmailTemplates(context.Context, RowSink) error
    ScanBackupKnowledgeBaseArticles(context.Context, RowSink) error
    ScanBackupAIResearchJobs(context.Context, RowSink) error
    ScanBackupAIAuditLogs(context.Context, RowSink) error
}

type RequestMeta struct {
    ActorID   pgtype.UUID
    IPAddress string
    UserAgent string
}

type AuditLogger interface {
    Log(context.Context, pgtype.UUID, string, pgtype.UUID, string, any, string, string) error
}

type Service struct {
    source  RowSource
    audit   AuditLogger
    pool    *pgxpool.Pool
    queries db.Querier
}

func NewService(source RowSource, audit AuditLogger, pool *pgxpool.Pool, queries db.Querier) *Service
func (s *Service) Export(ctx context.Context, dst io.Writer, meta RequestMeta) error

func requestMeta(r *http.Request, claims *auth.AccessClaims) RequestMeta
```

- [ ] **Step 1: Write failing export tests**

Create a fake `RowSource` that emits known JSON rows and a counting writer. Assert:

```go
func TestExportStreamsEveryTableWithoutAccumulator(t *testing.T) {
    source := newFakeRowSource(2)
    var out bytes.Buffer
    svc := NewService(source, nil, nil, nil)
    err := svc.Export(context.Background(), &out, RequestMeta{})
    require.NoError(t, err)

    var payload struct {
        Format string                       `json:"format"`
        Tables map[string][]json.RawMessage `json:"tables"`
    }
    require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
    require.Equal(t, "openlocalcrm-backup", payload.Format)
    require.Len(t, payload.Tables, 24)
    require.Len(t, payload.Tables["contacts"], 2)
    require.NotContains(t, out.String(), `"custom_fields":"e30="`)
}
```

Add a source-level test that the export service has no `db.Querier` `List*` dependency; `RowSource` is the only read boundary. Implement `newFakeRowSource` in the same `_test.go` file: each of its 24 methods emits the requested number of minimal objects such as `{"id":"00000000-0000-0000-0000-000000000001"}` and records that it was called.

- [ ] **Step 2: Run export tests and verify red**

```bash
go test ./internal/core/backup ./internal/server/handlers -run 'TestExport' -count=1
```

Expected: FAIL because the package and streamer do not exist.

- [ ] **Step 3: Add the 22 existing-table selectors to `backup.sql`**

Use this exact naming and ordering contract:

| Order | Table | Selector |
|---:|---|---|
| 1 | users | `ExportUsersRows` |
| 2 | companies | `ExportCompaniesRows` |
| 3 | workflows | `ExportWorkflowsRows` |
| 4 | contacts | `ExportContactsRows` |
| 5 | deals | `ExportDealsRows` |
| 6 | email_accounts | `ExportEmailAccountsRows` |
| 7 | todos | `ExportTodosRows` |
| 8 | refresh_tokens | `ExportRefreshTokensRows` |
| 9 | audit_logs | `ExportAuditLogsRows` |
| 10 | notifications | `ExportNotificationsRows` |
| 11 | email_messages | `ExportEmailMessagesRows` |
| 12 | email_attachments | `ExportEmailAttachmentsRows` |
| 13 | appointments | `ExportAppointmentsRows` |
| 14 | call_activities | `ExportCallActivitiesRows` |
| 15 | company_research | `ExportCompanyResearchRows` |
| 16 | workflow_runs | `ExportWorkflowRunsRows` |
| 17 | workflow_steps | `ExportWorkflowStepsRows` |
| 18 | notes | `ExportNotesRows` |
| 19 | intake_forms | `ExportIntakeFormsRows` |
| 20 | knowledge_base_articles | `ExportKnowledgeBaseArticlesRows` |
| 21 | ai_research_jobs | `ExportAIResearchJobsRows` |
| 22 | ai_audit_logs | `ExportAIAuditLogsRows` |

Each query is exactly the pattern:

```sql
-- name: ExportContactsRows :many
SELECT row_to_json(t) FROM contacts t ORDER BY t.id;
```

Order by `id` for every table, including text-PK `workflows`, `workflow_runs`, and `workflow_steps`.

- [ ] **Step 4: Implement handwritten package-`db` scanners**

Create `internal/db/backup_stream.go`:

```go
package db

import (
    "context"
    "encoding/json"
    "fmt"
)

func scanBackupRows(ctx context.Context, q *Queries, statement string, sink func(context.Context, json.RawMessage) error) error {
    rows, err := q.db.Query(ctx, statement)
    if err != nil {
        return err
    }
    defer rows.Close()
    for rows.Next() {
        var raw []byte
        if err := rows.Scan(&raw); err != nil {
            return err
        }
        if err := sink(ctx, json.RawMessage(append([]byte(nil), raw...))); err != nil {
            return err
        }
    }
    return rows.Err()
}

func (q *Queries) ScanBackupUsers(ctx context.Context, sink func(context.Context, json.RawMessage) error) error {
    if sink == nil {
        return fmt.Errorf("backup: nil row sink")
    }
    return scanBackupRows(ctx, q, exportUsersRows, sink)
}
```

Implement the 24 public methods listed in the `RowSource` interface. Each method calls `scanBackupRows` with the corresponding generated constant from the Step 3 selector table, plus `exportUserInvitesRows` and `exportEmailTemplatesRows`. Do not call the generated `:many` methods.

- [ ] **Step 5: Implement the demo row source**

Create `internal/db/demo/backup_stream.go`. Implement the same `backup.RowSource` interface by locking the relevant map once, iterating it, `json.Marshal`ing one row, and invoking the sink. Never build a `[]json.RawMessage`. Missing demo maps emit zero rows.

- [ ] **Step 6: Build the table registry and streaming envelope**

Create `internal/core/backup/tables.go` with the 24-entry order from spec §3.2 and this descriptor shape:

```go
type tableSpec struct {
    Name   string
    Stream func(context.Context, RowSink) error
}

var tableSpecs = []tableSpec{
    {Name: "users", Stream: scanUsers},
    {Name: "companies", Stream: scanCompanies},
}
```

Build the service with `rowSource, ok := cfg.DB.(backup.RowSource)`. Both `*db.Queries` and `*demo.InMemoryQuerier` implement that interface, so the router does not import the demo package. A failed assertion disables export with 501 rather than panicking. Pass `cfg.DB` as the static query dependency, `auditSvc` as the audit dependency, and `cfg.Pool` only for later scratch operations. `Export` writes metadata and each table boundary, then calls each source scanner. On error after headers start, return the error to the handler, which aborts with `http.ErrAbortHandler` rather than completing a success document.

- [ ] **Step 7: Replace the RAM-buffering handler**

Change the handler to:

```go
func (h *BackupHandler) Export(w http.ResponseWriter, r *http.Request) {
    claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
    if claims == nil || claims.Role != "ADMIN" {
        writeJSONError(w, http.StatusForbidden, "forbidden", "Nur Administratoren können Daten-Backups exportieren")
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Content-Disposition", `attachment; filename="openlocalcrm-backup.json"`)
    w.WriteHeader(http.StatusOK)
    if err := h.service.Export(r.Context(), w, requestMeta(r, claims)); err != nil {
        panic(http.ErrAbortHandler)
    }
}
```

Audit `BACKUP_EXPORT_STARTED` before the first row and `BACKUP_EXPORT_COMPLETED` or `BACKUP_EXPORT_FAILED` after the stream, with `auth.ClientIP(r)` and `r.UserAgent()`.

Implement the handler metadata helper in `internal/server/handlers/backup.go`:

```go
func requestMeta(r *http.Request, claims *auth.AccessClaims) backup.RequestMeta {
    var actorID pgtype.UUID
    if claims != nil {
        _ = actorID.Scan(claims.UserID.String())
    }
    return backup.RequestMeta{
        ActorID: actorID,
        IPAddress: auth.ClientIP(r),
        UserAgent: r.UserAgent(),
    }
}
```

- [ ] **Step 8: Stream the browser download**

Add `downloadToFile` to `web/src/api/client.ts`; it must attach CSRF/auth cookies, call `response.body.getReader()`, and write chunks to a blob without `response.json()` or `JSON.stringify`. Replace `handleDownloadBackup` with this helper and surface non-2xx JSON errors.

- [ ] **Step 9: Run export tests and web checks**

```bash
/Users/ewenwitte/go/bin/sqlc generate
go test ./internal/core/backup ./internal/server/handlers ./internal/db/... -run 'Export|Backup' -count=1
cd web && pnpm lint && pnpm typecheck
```

Expected: PASS.

---

### Task 3: Transactional Restore and Real Scratch Drill

**Files:**
- Modify: `internal/db/queries/backup.sql`
- Create: `internal/core/backup/scratch.go`
- Modify: `internal/core/backup/service.go`
- Modify: `internal/server/handlers/backup.go`
- Modify: `internal/server/router.go`
- Modify: `cmd/server/main.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Test: `internal/core/backup/restore_test.go`, `internal/core/backup/scratch_integration_test.go`, `internal/server/handlers/backup_test.go`

**Interfaces:**
- Consumes: Task 2 row source and table registry.
- Produces:

```go
type RestoreMode string
const (
    RestoreModeMerge   RestoreMode = "merge"
    RestoreModeReplace RestoreMode = "replace"
)

type RestoreResult struct {
    Mode     RestoreMode
    Counts   map[string]int64
    Committed bool
}

var (
    ErrRestoreRolledBack  = errors.New("backup restore rolled back")
    ErrRestoreAuditFailed = errors.New("backup committed but audit insert failed")
    ErrBackupUnavailable  = errors.New("PostgreSQL backup operations are unavailable")
)

func (s *Service) Restore(ctx context.Context, body io.Reader, mode RestoreMode, meta RequestMeta) (RestoreResult, error)
func (s *Service) Drill(ctx context.Context, meta RequestMeta) (DrillResult, error)
```

- [ ] **Step 1: Write failing restore/drill tests**

Unit tests must prove:

- invalid envelope/table/column fails before a transaction;
- merge upsert is idempotent;
- replace rollback leaves original rows;
- post-commit audit failure returns `ErrRestoreAuditFailed` with `Committed == true`;
- rollback returns `ErrRestoreRolledBack` with `Committed == false`;
- demo restore/drill return 501;
- a fake scratch manager records `CREATE SCHEMA`, copied-FK drops, scratch-FK additions, and guaranteed drop.

Use a build-tagged PostgreSQL test for actual FK enforcement:

```go
//go:build integration

func TestScratchDrillCopiesSeededRowsWithoutFalseZeroPass(t *testing.T) {
    dbURL := os.Getenv("TEST_DATABASE_URL")
    if dbURL == "" { t.Skip("TEST_DATABASE_URL not set") }
    // Seed at least one user, company, contact, and deal in public.
    // Run Drill. Require every seeded table's scratch count > 0 and equal to public.
    // Query pg_constraint and require no scratch FK references public.
    // Require the scratch schema is absent after success and injected failure.
}
```

- [ ] **Step 2: Run red tests**

```bash
go test ./internal/core/backup ./internal/server/handlers -run 'Restore|Drill' -count=1
```

Expected: FAIL because restore and scratch behavior do not exist.

- [ ] **Step 3: Add static restore statements**

Add `Upsert<Table>Row :exec` for the 22 `backup.sql` tables and `TruncateBackupTables :exec`. The truncate statement lists tables in reverse order and ends with `RESTART IDENTITY CASCADE`.

Use explicit `jsonb_to_record` casts. The mandatory non-JSONB casts are:

```sql
-- knowledge_base_articles
(r->>'embedding')::vector(1024)
(r->>'tags')::text[]

-- company_research
(r->>'industry_keywords')::text[]

-- enum-backed columns
(r->>'role')::user_role
(r->>'status')::user_status
(r->>'priority')::todo_status
(r->>'direction')::email_direction
```

Every upsert uses `ON CONFLICT (id) DO UPDATE` and updates every non-PK column. `email_templates` uses `ON CONFLICT (key)` in its own query file.

- [ ] **Step 4: Spool and validate before transaction work**

Implement a private `spoolBackup` using `os.CreateTemp` and `io.Copy`. In the handler, wrap the body before calling the service:

```go
const maxRestoreBytes = 512 << 20
r.Body = http.MaxBytesReader(w, r.Body, maxRestoreBytes)
result, err := h.service.Restore(r.Context(), r.Body, mode, requestMeta(r, claims))
```

After spooling, rewind the file and use `json.Decoder` token streaming to validate the envelope, 24-table allowlist, row object shape, and required primary keys. Reject unknown columns against a per-table allowlist derived from the static query contract. Delete the temp file in all paths.

- [ ] **Step 5: Implement merge and replace transactions**

Use a new package-`db` helper so services do not need the pool for normal transactions:

```go
var ErrTransactionsUnsupported = errors.New("db: transactions are unsupported")

func (q *Queries) Begin(ctx context.Context) (pgx.Tx, error) {
    beginner, ok := q.db.(interface {
        Begin(context.Context) (pgx.Tx, error)
    })
    if !ok {
        return nil, ErrTransactionsUnsupported
    }
    return beginner.Begin(ctx)
}
```

In the transaction, obtain it through `s.queries.(interface{ Begin(context.Context) (pgx.Tx, error) })`; return `ErrBackupUnavailable` when the assertion fails. Run `TruncateBackupTables` only for explicit replace, then execute upserts in the 24-entry order through `db.New(tx)`. On any error, roll back and return `ErrRestoreRolledBack`. On commit success, set `Committed = true` before audit.

- [ ] **Step 6: Make post-commit audit distinct**

After commit:

```go
backupEntityID := pgtype.UUID{Bytes: uuid.UUID{}, Valid: true}
if err := s.audit.Log(ctx, meta.ActorID, "BACKUP", backupEntityID, "BACKUP_RESTORE_COMPLETED", map[string]any{
    "mode": string(mode), "counts": result.Counts,
}, meta.IPAddress, meta.UserAgent); err != nil {
    return RestoreResult{Committed: true}, fmt.Errorf("%w: %v", ErrRestoreAuditFailed, err)
}
```

Handler responses:

- rollback: HTTP 500, `{"error":"restore_rolled_back","committed":false}`;
- post-commit audit failure: HTTP 500, `{"error":"restore_committed_audit_failed","committed":true}`.

Never retry the restore automatically; merge is idempotent but replace is not.

- [ ] **Step 7: Implement scratch clone and FK rewrite**

Add `Pool *pgxpool.Pool` to `server.Config` and pass `dbPool` from `main.go`; demo keeps it nil.

`Drill` performs:

1. unique schema name from crypto/rand;
2. `CREATE TABLE scratch.<name> (LIKE public.<name> INCLUDING ALL)` for all 24 tables;
3. query `pg_constraint` for public FK definitions;
4. drop any copied scratch FK before adding replacements;
5. rewrite `REFERENCES public.` to `REFERENCES scratch.` and add the FK to the scratch child;
6. acquire a source connection and create `db.New(sourceConn)` with effective schema `public`;
7. begin a separate target transaction, `SET LOCAL search_path TO scratch, public`, and create `db.New(targetTx)`;
8. stream source rows into target upserts;
9. compare nonzero seeded counts and enforce constraints;
10. `DROP SCHEMA ... CASCADE` in a deferred cleanup path.

- [ ] **Step 8: Add routes and honest UI**

Add `POST /api/v1/backup/restore` under the existing admin backup group. Keep drill POST bodyless. In `SettingsPage`, add a backup file input and explicit replace checkbox; upload with credentialed `fetch`, CSRF header, and the selected mode. Render `committed:false` and `committed:true` errors distinctly.

- [ ] **Step 9: Run tests**

```bash
go test ./internal/core/backup ./internal/server/handlers ./internal/db/... -count=1
```

With a disposable already-migrated PostgreSQL test database:

```bash
TEST_DATABASE_URL='postgres://...' go test -tags=integration ./internal/core/backup -run TestScratchDrill -count=1
```

Expected: PASS; scratch counts equal seeded public counts, no scratch FK references public, and scratch schema is gone.

- [ ] **Step 10: Commit Task 2**

```bash
git add internal/db/queries/backup.sql internal/db/*.sql.go internal/db/backup_stream.go \
  internal/db/demo internal/core/backup internal/server/handlers/backup.go \
  internal/server/handlers/backup_test.go internal/server/router.go cmd/server/main.go \
  web/src/api/client.ts web/src/pages/SettingsPage.tsx e2e
git commit -m "feat(backup): implement full streaming backup, restore, and real restore drill (B1, B2)"
```

---

### Task 4: Invite Issuance and Acceptance

**Files:**
- Create: `internal/core/invite/service.go`
- Create: `internal/server/handlers/invites.go`
- Modify: `internal/db/queries/invites.sql`
- Regenerate: `internal/db/invites.sql.go`, `internal/db/querier.go`
- Modify: `internal/db/demo/querier.go`
- Modify: `internal/server/handlers/users.go`
- Modify: `internal/server/router.go`
- Test: `internal/core/invite/service_test.go`, `internal/server/handlers/invites_test.go`, `internal/server/handlers/users_test.go`

**Interfaces:**
- Produces:

```go
type CreateUserInput struct {
    Email     string
    FirstName string
    LastName  string
    Role      string
}

type InviteView struct {
    Email     string    `json:"email"`
    FirstName string    `json:"first_name"`
    ExpiresAt time.Time `json:"expires_at"`
}

type IssuedInvite struct {
    Invite    db.UserInvite
    RawToken  string
    URL       string
    ExpiresAt time.Time
}

func (s *Service) Issue(ctx context.Context, actorID pgtype.UUID, req CreateUserInput) (IssuedInvite, error)
func (s *Service) Validate(ctx context.Context, rawToken string) (InviteView, error)
func (s *Service) Accept(ctx context.Context, rawToken, password string) (db.User, error)
```

- [ ] **Step 1: Write failing invite tests**

Cover: raw token differs from stored hash, 72-hour expiry, GET does not consume, POST activates and consumes, second POST fails, expired/revoked tokens fail, wrong password length fails, and issuance returns no email-sent claim.

- [ ] **Step 2: Run red tests**

```bash
go test ./internal/core/invite ./internal/server/handlers -run Invite -count=1
```

Expected: FAIL because the service and routes do not exist.

- [ ] **Step 3: Implement the service**

Generate 32 bytes with `crypto/rand`, encode with `base64.RawURLEncoding`, and hash with SHA-256 hex. Add `CreateInvitedUserWithInvite :one` to `invites.sql`: one CTE inserts the `INVITED` user with a discarded random password hash and then inserts the invite, returning the invite row. The handler loads the user by `invite.user_id`. Build the returned link from `DOMAIN` when set, otherwise from the request scheme/host, and always use path `/accept-invite?token=<raw-token>`. Never log or audit the raw token.

Add `AcceptUserInvite :one` as one atomic statement. It locks the active invite with `SELECT ... FOR UPDATE`, updates `consumed_at`, and updates the referenced user to the new Argon2id hash and `ACTIVE` in the same statement/transaction:

```sql
-- name: AcceptUserInvite :one
WITH locked AS (
    SELECT id, user_id
    FROM user_invites
    WHERE token_hash = sqlc.arg(token_hash)
      AND consumed_at IS NULL
      AND revoked_at IS NULL
      AND expires_at > NOW()
    FOR UPDATE
), consumed AS (
    UPDATE user_invites i
    SET consumed_at = NOW()
    FROM locked l
    WHERE i.id = l.id
    RETURNING l.user_id
)
UPDATE users u
SET password_hash = sqlc.arg(password_hash),
    status = 'ACTIVE',
    updated_at = NOW()
FROM consumed c
WHERE u.id = c.user_id
RETURNING u.*;
```

`Accept` hashes the new password and calls this query. Unknown/consumed/revoked/expired tokens return no row and a stable 4xx error. The demo implementation performs the same predicate and state changes under one in-memory lock.

- [ ] **Step 4: Add handlers and routes**

- Admin: `POST /api/v1/users/invite` returns `{user, invite_url, expires_at}`.
- Public: `GET /api/v1/auth/accept-invite?token=...` returns `{valid,email,first_name,expires_at}` without mutation.
- Public, CSRF-exempt, rate-limited: `POST /api/v1/auth/accept-invite` accepts `{token,password}`.

Audit `USER_INVITE_CREATED`, `USER_INVITE_ACCEPTED`, and rejected acceptance without raw-token material. Run `/Users/ewenwitte/go/bin/sqlc generate` after adding the two invite statements and implement their demo methods before compiling.

- [ ] **Step 5: Run invite tests**

```bash
go test ./internal/core/invite ./internal/server/handlers ./internal/server -run Invite -count=1
```

Expected: PASS.

---

### Task 5: Real Email Templates and TOTP Disable

**Files:**
- Create: `internal/core/emailtemplate/service.go`
- Create: `internal/server/handlers/email_templates.go`
- Modify: `internal/server/router.go`
- Modify: `web/src/context/AuthContext.tsx`
- Modify: `internal/server/router.go` `/me` response
- Test: `internal/core/emailtemplate/service_test.go`, `internal/server/handlers/email_templates_test.go`

**Interfaces:**
- Produces:

```go
type GeneratedTemplate struct {
    Name    string `json:"name"`
    Subject string `json:"subject"`
    Body    string `json:"body"`
}
func (s *Service) Generate(ctx context.Context, prompt string) (GeneratedTemplate, error)
```

- [ ] **Step 1: Write failing template and session tests**

Test CRUD persistence, unique-key conflict, AI JSON parsing, upstream failure as a typed error, and `/me` returning `totp_enabled` from the database user rather than local state.

- [ ] **Step 2: Run red tests**

```bash
go test ./internal/core/emailtemplate ./internal/server/handlers ./internal/server -run 'Template|MeTotp' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement template service and routes**

Implement admin-only:

- `GET /api/v1/email-templates`
- `POST /api/v1/email-templates`
- `PUT /api/v1/email-templates/{id}`
- `DELETE /api/v1/email-templates/{id}`
- `POST /api/v1/email-templates/generate`

Generation calls the real `ai.Gateway.Generate` with a system instruction requiring one JSON object with `name`, `subject`, and `body`. Invalid JSON or upstream failure returns 502/4xx; it never returns canned text.

- [ ] **Step 4: Expose TOTP state**

Add `totp_enabled` to `AuthContext.UserSession`. The `/me` handler loads the user and returns the real flag. Do not implement SP8a changes in this task.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/core/emailtemplate ./internal/server/handlers ./internal/server -run 'Template|MeTotp' -count=1
```

Expected: PASS.

---

### Task 6: Settings Frontend, Webhook Contract, E2E, and Final SP7 Gates

**Files:**
- Create: `web/src/pages/AcceptInvitePage.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `e2e/auth.spec.ts`
- Modify: `e2e/connectors.spec.ts`
- Modify: `e2e/live-simulation.spec.ts`
- Test: existing Go tests plus Playwright

**Interfaces:**
- Consumes: all Task 2–5 routes.
- Produces: no new backend interface.

- [ ] **Step 1: Update Playwright specs first**

Replace the fake invite-sent assertion with:

```ts
await expect(page.getByText(/Einladung erstellt/)).toBeVisible();
await expect(page.getByRole('textbox', { name: /Einladungslink/i })).toHaveValue(/\/accept-invite\?token=/);
```

Add a clean-context invite acceptance test. Replace the TOTP local-toggle assertion with a request assertion for `POST /api/v1/auth/totp/disable` and a server error case. Update the connector test to fill the empty token field and assert the request has `Authorization: Bearer ...` and no `X-Connector-Token`. Update the drill test to accept an honest 501 in demo mode.

- [ ] **Step 2: Run red Playwright tests**

```bash
cd web && pnpm exec playwright test --config=../playwright.config.ts ../e2e/auth.spec.ts ../e2e/connectors.spec.ts ../e2e/live-simulation.spec.ts --project=chromium
```

Expected: FAIL against the old SettingsPage/App behavior.

- [ ] **Step 3: Implement the invite page and route**

`AcceptInvitePage` loads the token from `useSearchParams`, calls GET validation, shows email/expiry, and posts the password. On success it displays a real confirmation and links to `/login`; it never sets an auth cookie itself because login remains the only session-establishing operation.

Add `/accept-invite` as a public route in `App.tsx`.

- [ ] **Step 4: Wire SettingsPage to real operations**

- Invite: display the returned link with copy button; remove "erfolgreich gesendet" text.
- Backup: stream download; upload a selected file with merge/replace mode; distinguish rollback from committed-audit failure.
- TOTP: initialize from `useAuth().user.totp_enabled`; disable through a password-confirmation modal calling the real endpoint; update state only on success.
- Templates: load DB rows, support create/edit/delete, preview AI output, and persist only after an explicit save.
- Webhook: initialize `connectorToken` to `''`, use a masked input with no demo placeholder, and send:

```ts
const payload = {
  first_name: 'Sabine',
  last_name: 'Mustermann',
  email: `sabine.mustermann+${Date.now()}@example.de`,
  phone: '+49 89 12345678',
  street: 'Teststraße 1',
  city: 'München',
  zip: '80331',
  deal_title: 'PV-Anlage Test',
  deal_value: '18500.00',
  source: 'settings-webhook-test',
};
const res = await fetch(webhookUrl, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${connectorToken}`,
  },
  body: JSON.stringify(payload),
});
```

- [ ] **Step 5: Run web and E2E checks**

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck && pnpm build
pnpm exec playwright test --config=../playwright.config.ts e2e/auth.spec.ts e2e/connectors.spec.ts e2e/live-simulation.spec.ts --project=chromium
```

Expected: PASS for the three SP7-affected specs. The full E2E suite is not waived; run it after the focused specs and report any pre-existing selector failures separately from SP7 regressions.

- [ ] **Step 6: Run the complete SP7 gate**

```bash
go test -race ./...
./scripts/coverage.sh --min 50.0 --diff 50.0
make check
test -L migrations && ls -l migrations
```

Expected: all commands exit 0; coverage thresholds pass; symlink remains `migrations -> internal/db/migrations`.

- [ ] **Step 7: Inspect and commit Task 3**

```bash
git status --short
git diff --check
git diff --stat
git log --oneline -10
git add internal/core/invite internal/core/emailtemplate internal/server/handlers \
  internal/server/router.go internal/db web/src e2e
git commit -m "feat(settings): add token invites, 2FA wiring, templates, and webhook test (B3-B6)"
```

- [ ] **Step 8: Verify the three-commit branch contract**

```bash
SP7_BASE=$(git merge-base main HEAD)
git log --oneline "$SP7_BASE"..HEAD
```

Before the first task, record `SP7_BASE` in the SDD ledger. Expected exactly the three SP7 implementation commits from Tasks 1, 3, and 6, with no SP1–SP6 rewrite.

---

## Plan Self-Review

- **Spec coverage:** B1 streaming/24 tables, B2 restore/drill/source-target split/post-commit audit, B3 token invites, B4 TOTP, B5 templates, B6 Bearer webhook, audit metadata, demo 501, CSV-independent backup download, tests, and three commits each map to a task.
- **Type consistency:** `RowSource`, `RequestMeta`, `RestoreMode`, `RestoreResult`, `IssuedInvite`, and `GeneratedTemplate` are defined before use. Handler constructors change only where Tasks 2–4 specify.
- **Migration safety:** no applied migration is edited; `00018` and `00019` are the only new migrations.
- **Memory:** the only request spool is a bounded temporary file; table rows are never accumulated as `[]T`.
- **Placeholder scan:** no TBD/TODO/implicit "similar to" steps remain; the table/query contract is explicit.
