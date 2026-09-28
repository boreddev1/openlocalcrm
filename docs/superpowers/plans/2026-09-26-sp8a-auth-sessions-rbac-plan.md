# SP8a: Authentication, Sessions & RBAC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make sessions durably revocable, detect refresh-token reuse as account compromise, and turn the approved RBAC role matrix plus stable resource ownership into enforced code.

**Architecture:** One migration adds `users.token_version`, `users.must_change_password`, refresh-token `family_id`/`used_at`, and UUID owner columns on contacts, companies, notes and appointments. Access tokens carry `token_version` as a **required** claim that the auth middleware compares against the database on every request, which replaces the process-local deny list. Refresh rotation marks the presented token used and inserts its successor in one transaction; a replay revokes the whole family and increments `token_version` in that same transaction, so a detected theft ends every live session. A permission map in `internal/auth/rbac.go` is the single source of truth for the role matrix, and `internal/server/handlers/authz.go` provides fail-closed claim extraction and a UUID-based ownership check.

**Tech Stack:** Go 1.26, Chi router, `jackc/pgx/v5`, sqlc, `golang-jwt/jwt/v5` (Ed25519), Argon2id, TOTP, React 18, Vite 6, TypeScript, Tailwind, `@tanstack/react-query`, Playwright (Chromium), PostgreSQL 16 + pgvector, River.

## Global Constraints

- The approved spec is binding: `docs/superpowers/specs/2026-09-26-sp8a-auth-sessions-rbac-design.md`. Read it before starting; this plan implements it and adds no new requirements.
- Findings in scope: F-03, F-06, F-07, F-08, F-09, F-10, F-11, F-14, F-34, F-43, F-45. Anything else (F-16/F-18/F-19, F-22/F-23, F-24/F-25, F-26, F-27, F-28/F-29, F-30, F-37/F-38/F-39, F-40, F-41, F-42) is **out of scope** and belongs to SP8b.
- Exactly one new migration: `internal/db/migrations/00020_sp8a_sessions_and_ownership.sql`. Never edit an applied migration; `00010_foreign_key_indexes.sql` stays byte-identical.
- The 1024-dimensional embedding contract is untouched.
- All SQL lives in `internal/db/queries/*.sql`; regenerate with `/Users/ewenwitte/go/bin/sqlc generate` and commit the generated `*.sql.go`, `querier.go`, and `models.go`. No ORM, no handwritten SQL outside the queries directory.
- `db.Querier` gains methods, so `internal/db/demo/querier.go` must implement every one of them; the `var _ db.Querier = (*InMemoryQuerier)(nil)` assertion must keep compiling.
- Every new column must be added to the SP7 backup contract: `internal/db/queries/backup.sql` export/upsert pairs **and** the per-table allow-lists in `internal/core/backup/tables.go`. A column missing from the allow-list is rejected by the restore validator, and one missing from the upsert is silently dropped.
- The `migrations` symlink must stay identical to `internal/db/migrations`; verify with `diff -r migrations/ internal/db/migrations/`.
- `token_version` is a **required** claim. A token without it is rejected with 401. Never default a missing claim to 0 — that would let every pre-upgrade token pass.
- Ownership is authorized only through a UUID column supplied by the caller. `notes.author` and `appointments.assigned_to VARCHAR(255)` are display-only and are never authorization inputs. Deals, todos and email accounts keep their existing UUID owner columns.
- An ownership denial is 403, not 404.
- Handlers keep the existing `writeJSONError(w, status, code, message)` pattern from `internal/server/handlers/ai.go:90`.
- The password policy is a minimum of 8 characters. Do not add a digit or symbol requirement; the backend does not enforce one.
- `make check` includes `scripts/test-backup-integration.sh`, which needs Docker. It **fails closed** if `TEST_DATABASE_URL` is exported without `TEST_DATABASE_URL_ALLOW_EXISTING=1`; run `TEST_DATABASE_URL= make check` if your shell has it set.
- 11 E2E specs fail on `main` before SP8a and are out of scope: `ai-assistant`, `auth` password policy, `automations` runs, `calendar-and-telephony` modal, `connectors`, `deals-kanban` ×2, `inbox` ×2, `offer-calculator`, `reports`. Do not fix them; do not let a new SP8a regression hide among them.
- Coverage gate: ≥ 50 % total and ≥ 50 % on changed lines, enforced by the pre-push hook.

## Commit Map

| # | Message | Tasks |
|---|---|---|
| 1 | `fix(auth): harden sessions, TOTP setup and password policy (SP8a)` | 1–8 |
| 2 | `fix(auth): enforce the RBAC role matrix and stable resource ownership (SP8a)` | 9–16 |
| 3 | `feat(ui): wire ownership, password nag and route guards with the live session test (SP8a)` | 17–23 |

Commit 1 carries the session work together with F-03 and F-08, because all three live in `internal/auth` and neither F-03 nor F-08 fits the message of commit 2 or 3. The message names all three rather than hiding two findings behind one headline.

**Execution order:** run the three groups sequentially. Each group ends with its own commit, its own `make check`, and its own review before the next group starts.

---

# Commit Group 1 — Session and token lifecycle

### Task 1: Migration 00020, generated code, demo storage, backup contract

**Files:**
- Create: `internal/db/migrations/00020_sp8a_sessions_and_ownership.sql`
- Modify: `internal/db/queries/sessions.sql`
- Modify: `internal/db/queries/users.sql`
- Modify: `internal/db/queries/contacts.sql`, `internal/db/queries/companies.sql`, `internal/db/queries/notes.sql`, `internal/db/queries/appointments.sql`
- Modify: `internal/db/queries/backup.sql`
- Regenerate: `internal/db/sessions.sql.go`, `internal/db/users.sql.go`, `internal/db/contacts.sql.go`, `internal/db/companies.sql.go`, `internal/db/notes.sql.go`, `internal/db/appointments.sql.go`, `internal/db/backup.sql.go`, `internal/db/querier.go`, `internal/db/models.go`
- Modify: `internal/db/demo/querier.go`
- Modify: `internal/core/backup/tables.go`
- Test: `internal/db/queries/sp8a_test.go`

**Interfaces:**
- Consumes: nothing. This task creates the schema every later task depends on.
- Produces: `db.User.TokenVersion int32`, `db.User.MustChangePassword bool`, `db.RefreshToken.FamilyID pgtype.UUID`, `db.RefreshToken.UsedAt pgtype.Timestamptz`, `db.Contact.OwnerUserID pgtype.UUID`, `db.Company.OwnerUserID pgtype.UUID`, `db.Note.UserID pgtype.UUID`, `db.Appointment.UserID pgtype.UUID`, plus the new `db.Querier` methods listed below.

- [ ] **Step 1: Write the failing test**

Create `internal/db/queries/sp8a_test.go`. It asserts the *contract* the later tasks rely on: the migration file exists, the queries are declared, and every new column appears in both the backup upsert and the restore allow-list. It is a source-level test because a real database is exercised later in Task 5.

```go
package queries_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
	"github.com/stretchr/testify/require"
)

// TestSP8aMigrationDeclaresTheSessionAndOwnershipColumns pins the migration
// contents. A column that is added to the schema but not to the backup contract
// is silently dropped on restore, so both sides are asserted here.
func TestSP8aMigrationDeclaresTheSessionAndOwnershipColumns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "00020_sp8a_sessions_and_ownership.sql"))
	require.NoError(t, err)
	migration := string(raw)

	for _, want := range []string{
		"ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE refresh_tokens ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid()",
		"ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ",
		"ALTER TABLE contacts ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE companies ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE notes ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE appointments ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL",
	} {
		require.Contains(t, migration, want)
	}
	// Goose Down marker: the migrator only executes the text before it.
	require.Contains(t, migration, "-- +goose Down")
	// LF only, no CRLF.
	require.NotContains(t, migration, "\r")
}

// TestSP8aOwnershipColumnsSurviveBackupRestore fails if a new column is missing
// from the restore allow-list, which the restore validator enforces.
func TestSP8aOwnershipColumnsSurviveBackupRestore(t *testing.T) {
	for table, columns := range map[string][]string{
		"users":           {"token_version", "must_change_password"},
		"refresh_tokens":  {"family_id", "used_at"},
		"contacts":        {"owner_user_id"},
		"companies":       {"owner_user_id"},
		"notes":           {"user_id"},
		"appointments":    {"user_id"},
	} {
		allow := backup.TableColumns(table)
		require.NotEmpty(t, allow, "table %s must be in the backup registry", table)
		for _, col := range columns {
			require.Contains(t, allow, col, "table %s is missing column %s in the restore allow-list", table, col)
		}
	}

	backupSQL, err := os.ReadFile(filepath.Join("..", "queries", "backup.sql"))
	require.NoError(t, err)
	for _, want := range []string{
		"r.token_version::integer",
		"r.must_change_password::boolean",
		"r.family_id::uuid",
		"r.used_at::timestamptz",
		"r.owner_user_id::uuid",
		"r.user_id::uuid",
	} {
		require.True(t, strings.Contains(string(backupSQL), want),
			"backup.sql must carry %q so restore does not drop the column", want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/db/queries/ -count=1`
Expected: FAIL — `no such file or directory` for the migration, and `undefined: backup.TableColumns` because that accessor does not exist yet.

- [ ] **Step 3: Write the migration**

Create `internal/db/migrations/00020_sp8a_sessions_and_ownership.sql`. Every other file in that directory is the pattern to copy, including the Goose Up/Down split.

```sql
-- +goose Up
-- SP8a: durable session revocation, refresh token families, and stable
-- resource ownership. See docs/superpowers/specs/2026-09-26-sp8a-auth-sessions-rbac-design.md

-- Access tokens carry token_version; a mismatch with the stored value
-- invalidates every token issued before the change.
ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0;

-- Set for the bootstrapped admin. Soft: reported on /me, cleared on change.
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- Refresh rotation becomes single-use. family_id groups a rotation chain;
-- used_at marks a token as spent so a replay is detectable.
ALTER TABLE refresh_tokens ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_unused
    ON refresh_tokens(user_id) WHERE used_at IS NULL;

-- Ownership moves from free text to a stable user id. deals.assigned_to and
-- todos.assigned_to are already UUIDs and need no change.
ALTER TABLE contacts ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE companies ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE notes ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE appointments ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_contacts_owner_user_id ON contacts(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_companies_owner_user_id ON companies(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes(user_id);
CREATE INDEX IF NOT EXISTS idx_appointments_user_id ON appointments(user_id);

-- Backfill where a source exists: notes.author and appointments.assigned_to
-- hold the e-mail of the author or assignee. Rows that match no user stay NULL,
-- which under the ownership rule means ADMIN-only writes until assigned.
UPDATE notes SET user_id = u.id
  FROM users u
 WHERE lower(notes.author) = lower(u.email) AND notes.user_id IS NULL;

UPDATE appointments SET user_id = u.id
  FROM users u
 WHERE lower(appointments.assigned_to) = lower(u.email) AND appointments.user_id IS NULL;

-- +goose Down
ALTER TABLE appointments DROP COLUMN IF EXISTS user_id;
ALTER TABLE notes DROP COLUMN IF EXISTS user_id;
ALTER TABLE companies DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE contacts DROP COLUMN IF EXISTS owner_user_id;
DROP INDEX IF EXISTS idx_refresh_tokens_user_unused;
DROP INDEX IF EXISTS idx_refresh_tokens_family;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS used_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS family_id;
ALTER TABLE users DROP COLUMN IF EXISTS must_change_password;
ALTER TABLE users DROP COLUMN IF EXISTS token_version;
```

- [ ] **Step 4: Add the queries**

Append to `internal/db/queries/sessions.sql`. `ConsumeRefreshToken` is the rotation primitive: it marks the token used and returns the family in one statement, and it returns **no row** for an already-used or expired token, which is what makes replay detectable.

First, extend the existing `CreateRefreshToken` so a rotation chain has a family. `db.CreateRefreshTokenParams` then carries a `FamilyID pgtype.UUID`, and `UserAgent`/`IpAddress` are `pgtype.Text` — not `sql.NullString`:

```sql
-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (
    user_id,
    token_hash,
    user_agent,
    ip_address,
    expires_at,
    family_id
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;
```

Then add the rotation and cleanup statements:

```sql
-- name: ConsumeRefreshToken :one
UPDATE refresh_tokens
SET used_at = NOW()
WHERE token_hash = $1
  AND used_at IS NULL
  AND expires_at > NOW()
RETURNING id, user_id, family_id, user_agent, ip_address, expires_at;

-- name: GetRefreshTokenByHash :one
-- Deliberately ignores expiry and used_at: the replay path needs to see a spent
-- token in order to detect the replay.
SELECT * FROM refresh_tokens
WHERE token_hash = $1
LIMIT 1;

-- name: DeleteRefreshTokenFamily :exec
DELETE FROM refresh_tokens
WHERE family_id = $1;

-- name: DeleteExpiredAndSpentRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW() OR (used_at IS NOT NULL AND used_at < NOW() - INTERVAL '30 days');
```

Append to `internal/db/queries/users.sql`:

```sql
-- name: GetUserTokenVersion :one
SELECT token_version FROM users
WHERE id = $1;

-- name: IncrementUserTokenVersion :one
-- Invalidates every access token already issued to this user.
UPDATE users
SET token_version = token_version + 1, updated_at = NOW()
WHERE id = $1
RETURNING token_version;

-- name: SetUserMustChangePassword :exec
UPDATE users
SET must_change_password = $2, updated_at = NOW()
WHERE id = $1;

-- name: ListUnownedContacts :many
SELECT * FROM contacts
WHERE owner_user_id IS NULL
ORDER BY created_at, id;

-- name: ListUnownedCompanies :many
SELECT * FROM companies
WHERE owner_user_id IS NULL
ORDER BY created_at, id;

-- name: SetContactOwner :one
UPDATE contacts
SET owner_user_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCompanyOwner :one
UPDATE companies
SET owner_user_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
```

- [ ] **Step 5: Add the new columns to the backup contract**

In `internal/db/queries/backup.sql`, extend these four statements. The `users` pair, for example, becomes:

```sql
-- name: ExportUsersRows :many
SELECT row_to_json(t) FROM users t ORDER BY t.id;

-- name: UpsertUsersRow :exec
INSERT INTO users (
    id, email, password_hash, first_name, last_name, role, status,
    totp_secret_encrypted, totp_enabled, last_login_at, created_at,
    updated_at, totp_last_used_step, token_version, must_change_password
) VALUES (
    sqlc.arg(id)::uuid, sqlc.arg(email)::text, sqlc.arg(password_hash)::text,
    sqlc.arg(first_name)::text, sqlc.arg(last_name)::text,
    sqlc.arg(role)::text, sqlc.arg(status)::text,
    sqlc.arg(totp_secret_encrypted)::text, sqlc.arg(totp_enabled)::boolean,
    sqlc.arg(last_login_at)::timestamptz, sqlc.arg(created_at)::timestamptz,
    sqlc.arg(updated_at)::timestamptz, sqlc.arg(totp_last_used_step)::bigint,
    sqlc.arg(token_version)::integer, sqlc.arg(must_change_password)::boolean
)
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, email text, password_hash text, first_name text, last_name text,
    role text, status text, totp_secret_encrypted text, totp_enabled text,
    last_login_at text, created_at text, updated_at text, totp_last_used_step text,
    token_version text, must_change_password text
)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    password_hash = EXCLUDED.password_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    role = EXCLUDED.role,
    status = EXCLUDED.status,
    totp_secret_encrypted = EXCLUDED.totp_secret_encrypted,
    totp_enabled = EXCLUDED.totp_enabled,
    last_login_at = EXCLUDED.last_login_at,
    updated_at = EXCLUDED.updated_at,
    totp_last_used_step = EXCLUDED.totp_last_used_step,
    token_version = EXCLUDED.token_version,
    must_change_password = EXCLUDED.must_change_password;
```

Apply the same treatment to `ExportRefreshTokensRows`/`UpsertRefreshTokensRow` (`family_id`, `used_at`) and to the `contacts`, `companies`, `notes` and `appointments` pairs (`owner_user_id`, `owner_user_id`, `user_id`, `user_id`). Use the same typed-`jsonb_to_record` shape; never `r->>'field'`.

Then add the new columns to the allow-lists in `internal/core/backup/tables.go`, and add the accessor the test uses:

```go
// TableColumns returns the restore allow-list for one backup table.
func TableColumns(table string) []string {
	for _, spec := range tableSpecs {
		if spec.Name == table {
			return spec.Columns
		}
	}
	return nil
}
```

- [ ] **Step 6: Regenerate and implement the demo methods**

```bash
/Users/ewenwitte/go/bin/sqlc generate
gofmt -s -w internal/db
```

Expect: exit 0, no diagnostics, and a modified `internal/db/querier.go` with the new methods. Then implement every one of them in `internal/db/demo/querier.go`, following the existing `notes`/`callActivities` pattern: take `q.mu.Lock()`, do the work, unlock. Required semantics:

- `ConsumeRefreshToken`: return `pgx.ErrNoRows` when the token is absent, expired, or already used; otherwise set `UsedAt` and return the row.
- `GetRefreshTokenByHash`: return the row regardless of expiry or used state; `pgx.ErrNoRows` when absent.
- `DeleteRefreshTokenFamily`: delete every token with that family.
- `GetUserTokenVersion`: return `pgx.ErrNoRows` for an unknown user.
- `IncrementUserTokenVersion`: increment and return; `pgx.ErrNoRows` for an unknown user.
- `SetUserMustChangePassword`, `SetContactOwner`, `SetCompanyOwner`: mirror the SQL; `SetContactOwner`/`SetCompanyOwner` return `pgx.ErrNoRows` for an unknown id.
- `ListUnownedContacts`/`ListUnownedCompanies`: only rows whose owner is `NULL`, ordered by `created_at` then `id`, mirroring `ORDER BY`.
- `DeleteExpiredAndSpentRefreshTokens`: delete expired tokens and spent tokens older than 30 days.

Verify: `go build ./...` and `go vet ./...` are clean, and the compile-time assertion still holds.

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/db/queries/ -count=1`
Expected: PASS.

- [ ] **Step 8: Run the package gates**

```bash
gofmt -s -l internal/db internal/core/backup
go test ./internal/db/... ./internal/core/backup/... -count=1
./scripts/test-backup-integration.sh
```

Expected: no unformatted files, all tests PASS, and the script reports `✅ Real PostgreSQL backup and invite statements verified`. The script's disposable fixture applies `00020` automatically; if the migration is malformed this is where it surfaces.

### Task 2: `token_version` as a required access-token claim

**Files:**
- Modify: `internal/auth/jwt.go`
- Test: `internal/auth/jwt_test.go`

**Interfaces:**
- Consumes: `db.User.TokenVersion int32` from Task 1.
- Produces: `AccessClaims.TokenVersion *int32`, `GenerateAccessToken(userID, email, role string, tokenVersion int32, privKey, duration) (string, error)`, and `ValidateAccessToken` that rejects a token without the claim.

- [ ] **Step 1: Write the failing test**

Append to `internal/auth/jwt_test.go`:

```go
// TestJWTTokenVersionIsRequired is the regression guard for the upgrade path: a
// token minted before SP8a has no token_version claim, and defaulting it to 0
// would let it pass the version check until it expired.
func TestJWTTokenVersionIsRequired(t *testing.T) {
	priv, pub := testKeyPair(t)

	withVersion, err := GenerateAccessToken(testUserID, "a@example.de", "ADMIN", 7, priv, time.Minute)
	require.NoError(t, err)
	claims, err := ValidateAccessToken(withVersion, pub)
	require.NoError(t, err)
	require.NotNil(t, claims.TokenVersion)
	require.Equal(t, int32(7), *claims.TokenVersion)

	// Sign the same claims without the claim, exactly as the pre-SP8a code did.
	legacy := AccessClaims{
		UserID: testUserID,
		Email:  "a@example.de",
		Role:   "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   testUserID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Minute)),
			Issuer:    "openlocalcrm",
			Audience:  jwt.ClaimStrings{"openlocalcrm-api"},
		},
	}
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, legacy).SignedString(priv)
	require.NoError(t, err)

	_, err = ValidateAccessToken(legacyToken, pub)
	require.ErrorIs(t, err, ErrInvalidToken, "a token without token_version must be rejected")
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/auth/ -run TestJWTTokenVersionIsRequired -count=1`
Expected: FAIL to compile — `GenerateAccessToken` has no `tokenVersion` parameter and `AccessClaims` has no `TokenVersion` field.

- [ ] **Step 3: Implement the claim**

In `internal/auth/jwt.go`, add the field and make it a pointer so a missing claim is distinguishable from version 0:

```go
type AccessClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	// TokenVersion mirrors users.token_version at issue time. It is a pointer
	// so a token minted before SP8a is rejected rather than treated as 0.
	TokenVersion *int32 `json:"token_version"`
	jwt.RegisteredClaims
}
```

Change the generator to take and set it:

```go
func GenerateAccessToken(userID uuid.UUID, email string, role string, tokenVersion int32, privKey ed25519.PrivateKey, duration time.Duration) (string, error) {
	now := time.Now().UTC()
	version := tokenVersion
	claims := AccessClaims{
		UserID:       userID,
		Email:        email,
		Role:         role,
		TokenVersion: &version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			Issuer:    "openlocalcrm",
			Audience:  jwt.ClaimStrings{"openlocalcrm-api"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privKey)
}
```

And reject the missing claim in the validator, after the existing `token.Valid` check:

```go
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.TokenVersion == nil {
		return nil, ErrInvalidToken
	}

	return claims, nil
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/auth/ -run 'TestJWT' -count=1`
Expected: PASS, including the pre-existing `TestJWTEd25519` once its call sites are updated in Task 3.

### Task 3: Version check in the auth middleware and at the SSE connect

**Files:**
- Modify: `internal/auth/middleware.go`
- Modify: `internal/server/router.go:129-156` (SSE handler) and `internal/server/router.go:314` (middleware wiring)
- Modify: `internal/auth/service.go:71-130` (delete the deny list and the grace cache)
- Test: `internal/auth/middleware_test.go`, `internal/auth/service_test.go`

**Interfaces:**
- Consumes: `GenerateAccessToken` from Task 2, `db.Querier` from Task 1.
- Produces:

```go
// TokenVersionVerifier resolves the current token_version for a user.
type TokenVersionVerifier interface {
	CurrentTokenVersion(ctx context.Context, userID pgtype.UUID) (int32, error)
}

func AuthMiddleware(pubKey ed25519.PublicKey, versions TokenVersionVerifier) func(http.Handler) http.Handler
```

- [ ] **Step 1: Write the failing test**

Append to `internal/auth/middleware_test.go`:

```go
type stubVersions struct {
	version int32
	err     error
	calls   int
}

func (s *stubVersions) CurrentTokenVersion(_ context.Context, _ pgtype.UUID) (int32, error) {
	s.calls++
	return s.version, s.err
}

// TestAuthMiddlewareRejectsAMismatchedTokenVersion is the core F-06 guarantee:
// a demoted or deactivated user must stop working immediately, not in 30 minutes.
func TestAuthMiddlewareRejectsAMismatchedTokenVersion(t *testing.T) {
	priv, pub := testKeyPair(t)
	token, err := GenerateAccessToken(testUserID, "a@example.de", "ADMIN", 1, priv, time.Minute)
	require.NoError(t, err)

	var reached bool
	handler := AuthMiddleware(pub, &stubVersions{version: 2})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, reached, "a request with a stale token_version must not reach the handler")
}

// TestAuthMiddlewareFailsClosedOnVerifierError: a database error must deny the
// request rather than serve it on stale knowledge, because every authorization
// decision downstream depends on this read.
func TestAuthMiddlewareFailsClosedOnVerifierError(t *testing.T) {
	priv, pub := testKeyPair(t)
	token, err := GenerateAccessToken(testUserID, "a@example.de", "ADMIN", 1, priv, time.Minute)
	require.NoError(t, err)

	var reached bool
	handler := AuthMiddleware(pub, &stubVersions{err: errors.New("connection refused")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, reached)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/auth/ -run 'TestAuthMiddleware' -count=1`
Expected: FAIL to compile — `AuthMiddleware` takes one argument and `stubVersions` is undefined.

- [ ] **Step 3: Implement the verifier and the check**

In `internal/auth/middleware.go`:

```go
// TokenVersionVerifier resolves the current token_version for a user.
// *db.Queries and demo.InMemoryQuerier both satisfy it.
type TokenVersionVerifier interface {
	CurrentTokenVersion(ctx context.Context, userID pgtype.UUID) (int32, error)
}

func AuthMiddleware(pubKey ed25519.PublicKey, versions TokenVersionVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ... existing token extraction unchanged ...

			claims, err := ValidateAccessToken(tokenStr, pubKey)
			if err != nil {
				http.Error(w, `{"error":"unauthorized","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// Every access token carries the token_version it was minted with.
			// A mismatch means the user's role, status or password changed after
			// this token was issued, so the token is dead. A verifier error also
			// denies: serving on stale knowledge would silently disable the RBAC
			// matrix, which reads the role from these claims.
			if versions != nil {
				stored, verr := versions.CurrentTokenVersion(r.Context(), pgtype.UUID{Bytes: claims.UserID, Valid: true})
				if verr != nil {
					slog.Error("auth: token_version lookup failed", "user_id", claims.UserID, "error", verr)
					http.Error(w, `{"error":"unauthorized","message":"session could not be verified"}`, http.StatusUnauthorized)
					return
				}
				if claims.TokenVersion == nil || *claims.TokenVersion != stored {
					http.Error(w, `{"error":"unauthorized","message":"session is no longer valid"}`, http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			// ... bearer flag handling unchanged ...
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

Delete the `IsTokenRevoked(tokenStr)` call from the same function; the deny list no longer exists.

In `internal/auth/service.go`, delete `revokedAccessTokens sync.Map`, `RevokeAccessToken`, `IsAccessTokenRevoked`, the `graceCache sync.Map`, and the `rotatedTokenEntry` type. Keep `RevokeRefreshToken`, which deletes a persisted row.

Add the querier method that satisfies the interface, in `internal/db/queries/users.sql`:

```sql
-- name: CurrentTokenVersion :one
SELECT token_version FROM users
WHERE id = $1;
```

Regenerate, then implement it in the demo querier alongside `GetUserTokenVersion` (it can share the same helper) and wire `AuthMiddleware(cfg.PubKey, cfg.DB)` in `internal/server/router.go:314`.

- [ ] **Step 4: Apply the same check to the SSE stream**

The hand-rolled SSE handler at `internal/server/router.go:129-156` validates the token itself. After `auth.ValidateAccessToken` succeeds, add the same version comparison and 401 on mismatch. It runs once at connect; Task 5 adds the periodic re-check.

- [ ] **Step 5: Fix the existing tests that assert the removed behavior**

`internal/auth/service_test.go:82-85` asserts that replaying an old refresh token inside the grace window returns the **identical** new pair. That is the F-07 behavior being removed, so the assertion is deleted in Task 4, not here. In this task, update the remaining `GenerateAccessToken` call sites in tests to pass a version, and remove any test that calls the deleted `RevokeAccessToken`/`IsAccessTokenRevoked` functions.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
gofmt -s -w internal/auth internal/db
go build ./...
go vet ./...
go test ./internal/auth/... ./internal/server/... -count=1
```

Expected: PASS, with no reference to the deleted symbols anywhere in the tree.

### Task 4: Transactional single-use rotation with family revocation

**Files:**
- Create: `internal/db/tx.go`
- Create: `internal/db/demo/tx.go`
- Create: `internal/db/demo/tx_test.go`
- Modify: `internal/auth/service.go` (`RefreshToken`, the replay path, `SessionRevoker`)
- Modify: `internal/db/demo/querier.go` (`CreateRefreshToken` must copy `FamilyID`)
- Modify: `internal/server/router.go` (wire the revoker)
- Test: `internal/auth/service_test.go`

**Interfaces:**
- Consumes: `ConsumeRefreshToken`, `GetRefreshTokenByHash`, `DeleteRefreshTokenFamily`, `IncrementUserTokenVersion`, `CurrentTokenVersion` from Task 1; `GenerateAccessToken` from Task 2.
- Produces:

```go
// internal/auth — the seam that keeps internal/auth free of any sse import.
type SessionRevoker interface {
	RevokeUser(userID uuid.UUID)
}

// internal/db and internal/db/demo — one transactional capability, two
// implementations, so the refresh path works identically in production and
// demo mode.
type TxRunner interface {
	RunTx(ctx context.Context, fn func(db.Querier) error) error
}
```

*Why `RunTx` and not the SP7 `Begin`:* `(*Queries).Begin` exists only on `*db.Queries`; `demo.InMemoryQuerier` has no pool, so a `Begin`-based rotation returns `ErrTransactionsUnsupported` for every demo refresh and the Playwright suite loses login rotation entirely. `RunTx` gives the demo the same all-or-nothing semantics by snapshotting under its own lock.

- [ ] **Step 1: Write the failing tests**

Append to `internal/auth/service_test.go`. Delete the existing grace-cache assertion at lines 82-85 — it encodes the behaviour being removed.

```go
// newTestAuthService returns a service, its querier, and the public key, so a
// test can drive the real middleware with a real version source.
func newTestAuthService(t *testing.T) (*AuthService, db.Querier, ed25519.PublicKey) {
	t.Helper()
	priv, pub := testKeyPair(t)
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")
	svc := NewAuthService(q, priv, pub, auth.NewRateLimiterForTest())
	return svc, q, pub
}

// querierVersions is the real version source the middleware uses in
// production; the test uses the same one rather than a constant.
type querierVersions struct{ q db.Querier }

func (v querierVersions) CurrentTokenVersion(ctx context.Context, id pgtype.UUID) (int32, error) {
	return v.q.CurrentTokenVersion(ctx, id)
}

// TestRefreshTokenIsSingleUse: rotating consumes the presented token. The old
// 10-second grace cache handed the new pair to a second presenter.
func TestRefreshTokenIsSingleUse(t *testing.T) {
	svc, _, _ := newTestAuthService(t)
	_, refresh, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.NoError(t, err)

	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.ErrorIs(t, err, ErrInvalidRefreshToken, "a spent refresh token must not be redeemable twice")
}

// TestReplayRejectsTheAccessTokenIssuedBeforeIt is the §10.4 guarantee. It
// asserts the 401 through the real middleware; checking only the stored counter
// would pass even if nothing consulted it.
func TestReplayRejectsTheAccessTokenIssuedBeforeIt(t *testing.T) {
	svc, q, pub := newTestAuthService(t)
	access, refresh, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	// Before the replay the token works.
	require.Equal(t, http.StatusOK, serveWithMiddleware(t, svc, q, pub, access).Code)

	// A legitimate rotation succeeds.
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.NoError(t, err)

	// Replaying the spent token from another client is a compromise.
	_, _, err = svc.RefreshToken(context.Background(), refresh, "10.0.0.9", "attacker")
	require.ErrorIs(t, err, ErrInvalidRefreshToken)

	// The access token minted before all of this is now dead.
	rec := serveWithMiddleware(t, svc, q, pub, access)
	require.Equal(t, http.StatusUnauthorized, rec.Code,
		"without the token_version bump the stolen access token would still be accepted")
}

// TestRotationFailureDoesNotConsumeTheToken: if anything inside the transaction
// fails, the presented token must remain usable. Committing used_at first and
// failing afterwards would turn the client's next legitimate attempt into a
// theft signal.
func TestRotationFailureDoesNotConsumeTheToken(t *testing.T) {
	svc, q, _ := newTestAuthService(t)
	_, refresh, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	// A querier whose CreateRefreshToken always fails: the successor insert
	// aborts, so used_at must roll back with it.
	svc.querier = &failingQuerier{Querier: q, failCreateRefreshToken: true}
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.Error(t, err)

	svc.querier = q
	// The original token still works, so the client was not punished.
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.NoError(t, err)
}

// TestDeactivatedUserRotationDoesNotConsumeTheToken covers the ordering rule
// for the status check specifically: it must run before the commit.
func TestDeactivatedUserRotationDoesNotConsumeTheToken(t *testing.T) {
	svc, q, _ := newTestAuthService(t)
	_, refresh, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	setUserStatus(t, q, testUserID, "DEACTIVATED")
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.Error(t, err)

	// Reactivating the user and retrying must work; the token was never consumed.
	setUserStatus(t, q, testUserID, "ACTIVE")
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.NoError(t, err)
}

// TestDemoRefreshRollsBackLikeProduction: the demo path must have the same
// all-or-nothing semantics, or demo mode would exercise a different code path
// than production.
func TestDemoRefreshRollsBackLikeProduction(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")
	before := countRefreshTokens(t, q)

	err := q.RunTx(context.Background(), func(txq db.Querier) error {
		if _, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-a")); err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)
	require.Equal(t, before, countRefreshTokens(t, q), "RunTx must restore the snapshot on error")

	require.NoError(t, q.RunTx(context.Background(), func(txq db.Querier) error {
		_, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-b"))
		return err
	}))
	require.Equal(t, before+1, countRefreshTokens(t, q), "a committed RunTx must keep its writes")
}

// TestDemoRunTxDoesNotDeadlock is the guard on the mutex. InMemoryQuerier.mu is
// a sync.RWMutex, which is not reentrant: a RunTx that holds it and then calls
// the callback with the original querier blocks forever on the first method the
// callback uses. The timeout turns that hang into a failure instead of a stuck
// suite.
func TestDemoRunTxDoesNotDeadlock(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")

	done := make(chan error, 1)
	go func() {
		done <- q.RunTx(context.Background(), func(txq db.Querier) error {
			// Both a read and a write, because the reads take RLock and the
			// writes take Lock.
			if _, err := txq.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true}); err != nil {
				return err
			}
			_, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-deadlock"))
			return err
		})
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("RunTx deadlocked: the callback must not re-enter the querier's own mutex")
	}
}

// TestDemoRunTxRollsBackTheAuditLogSlice pins the slice case. auditLogs is the
// one non-map collection, and it is the field a map-only snapshot would miss.
func TestDemoRunTxRollsBackTheAuditLogSlice(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	before := len(q.AuditLogsForTest())

	err := q.RunTx(context.Background(), func(txq db.Querier) error {
		if _, err := txq.CreateAuditLog(context.Background(), auditLogParams("written inside a failed transaction")); err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)
	require.Equal(t, before, len(q.AuditLogsForTest()), "a rolled-back transaction must not leave an audit row behind")

	require.NoError(t, q.RunTx(context.Background(), func(txq db.Querier) error {
		_, err := txq.CreateAuditLog(context.Background(), auditLogParams("committed"))
		return err
	}))
	require.Equal(t, before+1, len(q.AuditLogsForTest()), "a committed transaction keeps its audit row")
}

// TestEveryDemoCollectionIsCloned guards the shadow clone. A new map or slice
// field that is missing from cloneForTransaction and adoptFrom would be shared
// between the shadow and the original, so it would escape rollback silently.
// Both map and slice kinds are counted, not just maps.
func TestEveryDemoCollectionIsCloned(t *testing.T) {
	typ := reflect.TypeOf(demo.InMemoryQuerier{})
	want := 0
	for i := 0; i < typ.NumField(); i++ {
		switch typ.Field(i).Type.Kind() {
		case reflect.Map, reflect.Slice, reflect.Ptr, reflect.Interface, reflect.Array:
			want++
		}
	}
	covered := demo.TransactionCollectionFields()
	require.Equal(t, want, len(covered),
		"every map or slice field on InMemoryQuerier needs a clone and adopt entry")
	require.Len(t, covered, len(uniqueStrings(covered)), "TransactionCollectionFields must not list a field twice")
}
```

Helper constructors and accessors used above go in the same file when the task runs, and each is stated there: `failingQuerier` (Step 5, which must implement `RunTx` itself), `setUserStatus`, `countRefreshTokens`, `refreshParams`, `auditLogParams`, `uniqueStrings`, `serveWithMiddleware`, `seedTestUser` (writes a user row with an Argon2id hash), and `demo.AuditLogsForTest()` (an accessor in `internal/db/demo/tx.go` returning `len(q.auditLogs)` under the read lock, alongside `TransactionCollectionFields`).

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/auth/ -run 'TestRefreshTokenIsSingleUse|TestReplayRejects|TestRotationFailure|TestDeactivatedUser' -count=1
go test ./internal/db/demo/ -run 'TestDemoRefresh|TestEveryDemoMap' -count=1
```

Expected: FAIL to compile — `RunTx`, `TxRunner`, `SessionRevoker`, and `TokenHash` on the params do not exist yet; and `TestRefreshTokenIsSingleUse` currently fails at runtime because the second presenter still succeeds.

- [ ] **Step 3: Implement `RunTx` for PostgreSQL**

Create `internal/db/tx.go`:

```go
package db

import "context"

// RunTx executes fn with a transactional Querier: either every statement
// commits or none does. It exists so services can group writes without
// reaching for the pool, and so the same call works against the in-memory demo
// querier, which implements RunTx by snapshot and restore.
func (q *Queries) RunTx(ctx context.Context, fn func(Querier) error) error {
	tx, err := q.Begin(ctx)
	if err != nil {
		return err
	}
	// The rollback is deferred immediately after Begin, before any work, so a
	// failure on any path releases the transaction. After a successful Commit it
	// is a no-op.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 4: Implement `RunTx` for the demo querier without deadlocking**

`InMemoryQuerier.mu` is a `sync.RWMutex` and every method takes it. `sync.RWMutex` is **not reentrant**, so `RunTx` must not hold `q.mu` and then call `fn(q)`: the callback's `CreateRefreshToken` or `GetUserByID` would block on the same mutex forever and every demo refresh would hang. The lock guards the snapshot; the callback runs against a **shadow clone** that owns its own mutex and its own copies of the data.

`InMemoryQuerier` has 18 map fields and one slice field (`auditLogs`), so a `maps.Clone`-only snapshot would silently fail to roll back the audit log — the append-only slice is the field most likely to be written inside a transaction.

Create `internal/db/demo/tx.go`:

```go
package demo

import (
	"context"
	"maps"
	"slices"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// TxRunner is the transactional capability a service can depend on. It is
// declared in both packages so either can be satisfied and asserted at compile
// time, and so internal/auth depends on neither pgx nor the demo package.
type TxRunner interface {
	RunTx(ctx context.Context, fn func(db.Querier) error) error
}

// RunTx gives the in-memory querier all-or-nothing semantics without a pool.
//
// It deliberately does not call fn(q). q.mu is a sync.RWMutex and every method
// takes it, so re-entering it from inside the callback would deadlock. Instead
// the callback runs against a shadow clone with its own mutex and its own copy
// of every map and slice. The original is untouched until the callback
// succeeds, so rollback is exact and needs no restore step, and no concurrent
// reader can observe a half-applied transaction.
func (q *InMemoryQuerier) RunTx(ctx context.Context, fn func(db.Querier) error) error {
	// Held for the whole callback so the transaction is serialised against
	// other writers and against itself; never re-entered, because fn works on
	// the shadow.
	q.mu.Lock()
	defer q.mu.Unlock()

	shadow := q.cloneForTransaction()
	if err := fn(shadow); err != nil {
		// q was never written, so there is nothing to restore.
		return err
	}
	q.adoptFrom(shadow)
	return nil
}

// cloneForTransaction builds the shadow. It constructs a new value rather than
// copying *q, because copying the struct would copy the mutex and trip
// go vet's copylocks check.
func (q *InMemoryQuerier) cloneForTransaction() *InMemoryQuerier {
	return &InMemoryQuerier{
		users:          maps.Clone(q.users),
		companies:      maps.Clone(q.companies),
		contacts:       maps.Clone(q.contacts),
		deals:          maps.Clone(q.deals),
		todos:          maps.Clone(q.todos),
		notifications:  maps.Clone(q.notifications),
		emailAccounts:  maps.Clone(q.emailAccounts),
		emailMessages:  maps.Clone(q.emailMessages),
		emailAttachs:   maps.Clone(q.emailAttachs),
		refreshTokens:  maps.Clone(q.refreshTokens),
		appointments:   maps.Clone(q.appointments),
		notes:          maps.Clone(q.notes),
		callActivities: maps.Clone(q.callActivities),
		kbArticles:     maps.Clone(q.kbArticles),
		aiResearchJobs: maps.Clone(q.aiResearchJobs),
		workflows:      maps.Clone(q.workflows),
		workflowRuns:   maps.Clone(q.workflowRuns),
		userInvites:    maps.Clone(q.userInvites),
		emailTemplates: maps.Clone(q.emailTemplates),
		// auditLogs is a slice, not a map: a map-only snapshot would leave every
		// audit row written inside an aborted transaction in place.
		auditLogs: slices.Clone(q.auditLogs),
	}
}

// adoptFrom copies the shadow's contents into q. The caller holds q.mu.
func (q *InMemoryQuerier) adoptFrom(s *InMemoryQuerier) {
	q.users = s.users
	q.companies = s.companies
	q.contacts = s.contacts
	q.deals = s.deals
	q.todos = s.todos
	q.notifications = s.notifications
	q.emailAccounts = s.emailAccounts
	q.emailMessages = s.emailMessages
	q.emailAttachs = s.emailAttachs
	q.refreshTokens = s.refreshTokens
	q.appointments = s.appointments
	q.notes = s.notes
	q.callActivities = s.callActivities
	q.kbArticles = s.kbArticles
	q.aiResearchJobs = s.aiResearchJobs
	q.workflows = s.workflows
	q.workflowRuns = s.workflowRuns
	q.userInvites = s.userInvites
	q.emailTemplates = s.emailTemplates
	q.auditLogs = s.auditLogs
}

// TransactionCollectionFields returns the names of the reference-typed fields
// that cloneForTransaction and adoptFrom must both cover. It exists for the
// coverage test; keep it in step with the two functions above.
func TransactionCollectionFields() []string {
	return []string{
		"users", "companies", "contacts", "deals", "todos", "notifications",
		"emailAccounts", "emailMessages", "emailAttachs", "refreshTokens",
		"appointments", "notes", "callActivities", "kbArticles",
		"aiResearchJobs", "workflows", "workflowRuns", "userInvites",
		"emailTemplates", "auditLogs",
	}
}
```

`maps.Clone` of a nil map returns nil, and the demo constructors initialise every field, so a shadow is always fully usable. Add the compile-time assertion next to the existing `var _ db.Querier = (*InMemoryQuerier)(nil)`: `var _ TxRunner = (*InMemoryQuerier)(nil)`.

Also make the demo's `CreateRefreshToken` copy `FamilyID` from the params; it currently drops it, which would leave every demo rotation in an empty family.

- [ ] **Step 5: Write the failure-injecting querier correctly**

An embedded `db.Querier` promotes the inner querier's `RunTx`, which hands the callback the **real** querier. The injection would then be dead code and the rollback test would pass for the wrong reason, so the wrapper implements `RunTx` itself and passes `f` to `fn`:

```go
// failingQuerier injects a failure into one method. It implements RunTx itself
// and passes *itself* to the callback: embedding db.Querier would promote the
// inner querier's RunTx, the callback would receive the real querier, and the
// injected failure would never fire.
type failingQuerier struct {
	db.Querier
	failCreateRefreshToken bool
}

func (f *failingQuerier) RunTx(ctx context.Context, fn func(db.Querier) error) error {
	runner, ok := f.Querier.(demo.TxRunner)
	if !ok {
		return db.ErrTransactionsUnsupported
	}
	return runner.RunTx(ctx, fn)
}

func (f *failingQuerier) CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error) {
	if f.failCreateRefreshToken {
		return db.RefreshToken{}, errors.New("injected CreateRefreshToken failure")
	}
	return f.Querier.CreateRefreshToken(ctx, arg)
}
```

`TestRotationFailureDoesNotConsumeTheToken` additionally asserts that the injection actually fired, by checking that the healthy path through the same wrapper does **not** error — otherwise a wrapper that silently never injects would make the test pass vacuously:

```go
	svc.querier = &failingQuerier{Querier: q, failCreateRefreshToken: false}
	_, _, err = svc.RefreshToken(context.Background(), refresh, "127.0.0.1", "test")
	require.NoError(t, err, "the injection itself must be the only difference")
```

- [ ] **Step 6: Implement the rotation with the correct ordering**

Replace `RefreshToken` in `internal/auth/service.go`. Everything that can fail runs **inside** the transaction, so a failure consumes nothing:

```go
func (s *AuthService) RefreshToken(ctx context.Context, rawRefreshToken, ip, userAgent string) (string, string, error) {
	hash := hashToken(rawRefreshToken)

	existing, err := s.querier.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrInvalidRefreshToken
		}
		return "", "", err
	}
	// An expired token is treated as unknown. Revoking a family because an old
	// cookie was replayed would punish the user for a stale cookie.
	if time.Now().After(existing.ExpiresAt.Time) {
		return "", "", ErrInvalidRefreshToken
	}
	if existing.UsedAt.Valid {
		s.handleReplay(ctx, existing, ip, userAgent)
		return "", "", ErrInvalidRefreshToken
	}

	var accessToken, newRefresh string
	err = s.runTx(ctx, func(q db.Querier) error {
		consumed, err := q.ConsumeRefreshToken(ctx, hash)
		if err != nil {
			// Another request consumed it between the read and here. That is a
			// race, not a theft, so roll back and let the caller retry.
			return ErrInvalidRefreshToken
		}
		// The status check runs before the commit. Checking it afterwards would
		// leave the old token consumed while the client got an error, so the
		// next attempt would look like a replay.
		user, err := q.GetUserByID(ctx, consumed.UserID)
		if err != nil {
			return err
		}
		if user.Status != "ACTIVE" {
			return ErrUserNotActive
		}
		if newRefresh, err = generateRefreshToken(); err != nil {
			return err
		}
		if _, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID:    consumed.UserID,
			TokenHash: hashToken(newRefresh),
			UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
			IpAddress: pgtype.Text{String: ip, Valid: ip != ""},
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(refreshTokenDuration), Valid: true},
			FamilyID:  consumed.FamilyID,
		}); err != nil {
			return err
		}
		// The access token is signed before the commit for the same reason: a
		// signing failure must not leave the rotation half applied.
		accessToken, err = s.issueAccessToken(user)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return accessToken, newRefresh, nil
}
```

`issueAccessToken(user db.User)` reads `user.TokenVersion` and calls `GenerateAccessToken` from Task 2.

- [ ] **Step 7: Implement the replay path with the correct ordering**

```go
// SessionRevoker terminates the live streams of a user whose session state was
// invalidated. It is an interface so internal/auth does not import sse; the
// router passes the real hub in.
type SessionRevoker interface {
	RevokeUser(userID uuid.UUID)
}

// handleReplay treats a spent refresh token as account compromise. Revoking the
// family alone would leave the stolen access token valid for 30 minutes, so the
// user's token_version is incremented in the same transaction: every access
// token already issued dies with it.
func (s *AuthService) handleReplay(ctx context.Context, spent db.RefreshToken, ip, userAgent string) {
	reason := "refresh_token_reuse"
	if spent.UserAgent.String != userAgent || spent.IpAddress.String != ip {
		reason = "refresh_token_reuse_other_client"
	}

	revoked := s.runTx(ctx, func(q db.Querier) error {
		if err := q.DeleteRefreshTokenFamily(ctx, spent.FamilyID); err != nil {
			return err
		}
		// The error is not ignored: a family revocation without the version bump
		// would leave the stolen access token alive.
		_, err := q.IncrementUserTokenVersion(ctx, spent.UserID)
		return err
	}) == nil

	// The streams are closed only after a successful commit, so a failed
	// transaction cannot disconnect a user whose session was never invalidated.
	if revoked && s.revoker != nil {
		s.revoker.RevokeUser(uuid.UUID(spent.UserID.Bytes))
	}

	// The audit is unconditional: a detected replay must be visible even when
	// the revocation itself failed.
	s.audit.Log(ctx, spent.UserID, "AUTH", pgtype.UUID{}, "REFRESH_TOKEN_REUSE", map[string]any{
		"reason":         reason,
		"family_id":      spent.FamilyID.String(),
		"revoked":        revoked,
		"detected_ip":    ip,
		"detected_agent": userAgent,
	}, ip, userAgent)
}

// runTx adapts the querier to the transactional capability. A querier without
// one is reported honestly instead of silently running without a transaction,
// which would make a partial rotation look successful.
func (s *AuthService) runTx(ctx context.Context, fn func(db.Querier) error) error {
	runner, ok := s.querier.(TxRunner)
	if !ok {
		return db.ErrTransactionsUnsupported
	}
	return runner.RunTx(ctx, fn)
}
```

Add the fields `revoker SessionRevoker` and `audit` to `AuthService`, and a nil-safe setter used by the router:

```go
// WithSessionRevoker attaches the live-stream revoker. It is optional so tests
// and the launcher can build a service without a hub.
func (s *AuthService) WithSessionRevoker(r SessionRevoker) *AuthService {
	s.revoker = r
	return s
}
```

- [ ] **Step 8: Wire the revoker in the router**

In `internal/server/router.go`, right after the auth service is constructed: `authSvc = authSvc.WithSessionRevoker(cfg.SSEHub)` — only when `cfg.SSEHub != nil`, since a nil hub must not be stored.

- [ ] **Step 9: Run the tests to verify they pass**

```bash
gofmt -s -w internal/auth internal/db
go build ./...
go vet ./...
go test ./internal/auth/... ./internal/db/... -count=1
```

Expected: PASS, including the demo rollback tests.

- [ ] **Step 10: Mutation-verify the three guarantees**

Remove the `used_at IS NULL` predicate from `ConsumeRefreshToken` and confirm `TestRefreshTokenIsSingleUse` fails. Then remove the `IncrementUserTokenVersion` call from `handleReplay` and confirm `TestReplayRejectsTheAccessTokenIssuedBeforeIt` fails — the access token issued before the replay is still accepted, and that survival is what fails the test. Then move the `Status` check after the commit and confirm `TestDeactivatedUserRotationDoesNotConsumeTheToken` fails. Then replace the shadow clone with `fn(q)` in the demo `RunTx` and confirm `TestDemoRunTxDoesNotDeadlock` fails on its timeout rather than hanging the suite. Restore all four.

### Task 5: Logout-everywhere and SSE stream revocation

**Files:**
- Modify: `internal/sse/hub.go`
- Create: `internal/server/handlers/session.go`
- Modify: `internal/server/router.go`
- Test: `internal/sse/hub_test.go`, `internal/server/handlers/session_test.go`

**Interfaces:**
- Consumes: the `auth.SessionRevoker` seam from Task 4 — `*sse.Hub` is what satisfies it, and Task 4 already wired it into the router, so this task only has to make the hub conform.
- Produces:

```go
// internal/sse — satisfies auth.SessionRevoker
func (h *Hub) RevokeUser(userID uuid.UUID)
func (h *Hub) ServeHTTPFor(w http.ResponseWriter, r *http.Request, userID uuid.UUID)

// internal/server/handlers
type SessionHandler struct{ Auth *auth.AuthService; Hub *sse.Hub }
func (h *SessionHandler) LogoutAll(w http.ResponseWriter, r *http.Request)
```

- [ ] **Step 1: Write the failing test**

```go
// TestHubRevokeUserClosesOnlyThatUsersStreams
func TestHubRevokeUserClosesOnlyThatUsersStreams(t *testing.T) {
	hub := NewHub()
	alice, _ := uuid.Parse("11111111-1111-1111-1111-111111111111")
	bob, _ := uuid.Parse("22222222-2222-2222-2222-222222222222")

	aliceCh := hub.register(alice)
	bobCh := hub.register(bob)
	require.Equal(t, 2, hub.ClientCount())

	hub.RevokeUser(alice)

	_, open := <-aliceCh
	require.False(t, open, "alice's stream must be closed")
	_ = bobCh
	require.Equal(t, 1, hub.ClientCount(), "bob's stream must survive")
}

// TestHubSatisfiesTheAuthRevokerSeam pins the contract Task 4 depends on.
func TestHubSatisfiesTheAuthRevokerSeam(t *testing.T) {
	var _ auth.SessionRevoker = NewHub()
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/sse/ -run 'TestHubRevokeUser|TestHubSatisfies' -count=1`
Expected: FAIL to compile — `register` and `RevokeUser` are undefined, and `*Hub` does not yet satisfy `auth.SessionRevoker`.

- [ ] **Step 3: Implement the keyed hub**

Change `clients map[chan Event]bool` to `clients map[chan Event]uuid.UUID`, move the body of the existing `ServeHTTP` into `ServeHTTPFor(w, r, userID)` so the hub knows whose stream it is, keep `ServeHTTP` as a thin wrapper passing the zero id for the unauthenticated case, and add:

```go
// register records a subscriber's channel against its user id and returns the
// channel, so a test can observe the close.
func (h *Hub) register(userID uuid.UUID) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = userID
	return ch
}

// RevokeUser closes every stream belonging to userID. A role or status change
// must terminate a long-lived stream immediately, not at token expiry.
func (h *Hub) RevokeUser(userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, owner := range h.clients {
		if owner == userID {
			close(ch)
			delete(h.clients, ch)
		}
	}
}
```

`Broadcast` must skip a deleted channel. Because `close` happens under the write lock and `Broadcast` holds the read lock, the two cannot race. Update the SSE route in `internal/server/router.go:129-156` to call `ServeHTTPFor` with the user id it already resolved, and to run the 60-second `token_version` re-check from Step 5.

- [ ] **Step 4: Add the logout-everywhere route**

In `internal/server/handlers/session.go`, `LogoutAll` requires the claims fail-closed, calls `IncrementUserTokenVersion`, deletes every refresh token for the user, and calls `Hub.RevokeUser`:

```go
func (h *SessionHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	userID := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	if err := h.Auth.RevokeAllSessions(r.Context(), userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "revoke_failed", "Sitzungen konnten nicht widerrufen werden")
		return
	}
	h.Hub.RevokeUser(claims.UserID)
	handlers.ClearAuthCookies(w, r)
	writeJSONError(w, http.StatusOK, "ok", "Alle Sitzungen beendet")
}
```

Add `(*AuthService).RevokeAllSessions` (increment the version, delete the refresh tokens, notify the SSE hub) and the route `POST /api/v1/auth/logout-all` inside the protected group next to `/auth/logout`.

- [ ] **Step 5: Add the 60-second SSE re-check**

Inside the stream loop in `ServeHTTP`, add a `time.Ticker` at 60 seconds that re-reads `token_version` for the connecting user and returns when it no longer matches. This is the backstop for a change made outside this process, such as a direct database edit.

- [ ] **Step 6: Run the gates**

```bash
gofmt -s -w internal/sse internal/server/handlers
go test ./internal/sse/... ./internal/server/... -count=1
go vet ./...
```

Expected: PASS.

### Task 6: F-43 — no tokens in response bodies, cookie-only refresh

**Files:**
- Modify: `internal/server/handlers/auth.go` (`LoginResponse`, `Login`, `Refresh`, `RefreshRequest`)
- Modify: `internal/auth/service.go` (`LoginResult`)
- Test: `internal/server/handlers/auth_test.go`, `internal/server/demo_e2e_test.go:73-84`

**Interfaces:**
- Produces: `LoginResponse{User *UserResponse; MustChangePassword bool}` with no `token` field; `POST /api/v1/auth/refresh` reads the cookie only.

- [ ] **Step 1: Write the failing test**

```go
// TestLoginResponseCarriesNoToken: the cookie is the only channel. A raw token
// in a response body is readable by anything that can see the body.
func TestLoginResponseCarriesNoToken(t *testing.T) {
	h, _ := newTestAuthHandler(t)
	rec := performJSON(t, h, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "a@example.de", "password": "Str0ng!Pass",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, rec.Result().Cookies())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, body, "token")
	require.NotContains(t, body, "refresh_token")
}

// TestRefreshRejectsABodyToken: once the response carries no token, accepting a
// body token would strand a non-browser client with no way to continue.
func TestRefreshRejectsABodyToken(t *testing.T) {
	h, _ := newTestAuthHandler(t)
	rec := performJSON(t, h, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": "whatever",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/server/handlers/ -run 'TestLoginResponseCarriesNoToken|TestRefreshRejectsABodyToken' -count=1`
Expected: FAIL — the body still carries `token`, and a body token is accepted.

- [ ] **Step 3: Remove the fields**

Delete `Token` and `RefreshToken` from `LoginResponse` and from `LoginResult`, delete the `RefreshRequest` body field, and change `Refresh` to read only the `refresh_token` cookie, returning 400 when it is absent. Cookies stay exactly as they are: `access_token` HttpOnly Lax on `/`, `refresh_token` HttpOnly Strict on `/api/v1/auth`.

- [ ] **Step 4: Fix the tests that read the removed field**

`internal/server/demo_e2e_test.go:73-84` asserts `loginData.Token != ""`. Rewrite it to assert the `Set-Cookie` headers and that the body has no `token` key. Check `e2e/auth.setup.ts` and any Playwright storage-state helper for a stored token; if the suite authenticates by cookie it is unaffected, and if it reads the JSON body it must switch to `request.storageState()`.

- [ ] **Step 5: Run the gates**

```bash
gofmt -s -w internal/server/handlers
go test ./internal/server/... -count=1
```

Expected: PASS.

### Task 7: F-34 — bootstrap secret and the `must_change_password` flag

**Files:**
- Modify: `cmd/server/main.go:68-118` (`bootstrapAdminUser`)
- Modify: `internal/server/handlers/auth.go` (`/me` response)
- Test: `internal/server/handlers/invites_templates_routes_test.go` (the `/me` assertions live there), `internal/launcher/engine_test.go` if it asserts the log line

**Interfaces:**
- Produces: `writeBootstrapSecret(storagePath, email, password string) error`, and `must_change_password` in the `/me` body.

- [ ] **Step 1: Write the failing test**

```go
// TestBootstrapPasswordIsNotLoggedAndLandsInAFile: the secret must not reach the
// process log, and the file that holds it must be 0600.
func TestBootstrapPasswordIsNotLoggedAndLandsInAFile(t *testing.T) {
	dir := t.TempDir()
	var logs bytes.Buffer
	log.SetOutput(&logs)

	secret := "0123456789abcdef0123456789abcdef"
	require.NoError(t, writeBootstrapSecret(dir, "admin@example.de", secret))

	require.NotContains(t, logs.String(), secret, "the bootstrap password must never be logged")

	path := filepath.Join(dir, "bootstrap-admin.txt")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), secret)
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./cmd/server/ -run TestBootstrapPassword -count=1`
Expected: FAIL to compile — `writeBootstrapSecret` does not exist.

- [ ] **Step 3: Implement the file write and delete the log lines**

```go
// writeBootstrapSecret stores a generated administrator password in the storage
// directory with owner-only permissions and returns its path. The value is never
// written to the process log.
func writeBootstrapSecret(storagePath, email, password string) error {
	path := filepath.Join(storagePath, "bootstrap-admin.txt")
	body := fmt.Sprintf("email=%s\npassword=%s\nwritten=%s\n", email, password, time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(path, []byte(body), 0o600)
}
```

In `bootstrapAdminUser`, remove the four `log.Printf` lines that print the password, call `writeBootstrapSecret` instead, and log only the path plus a reminder. Set `must_change_password = TRUE` on the inserted admin row.

Delete the file in `ChangePassword` after a successful update, and add a test asserting the deletion.

- [ ] **Step 4: Report the flag on `/me`**

The `/me` handler at `internal/server/router.go:321-351` reads the user row for `totp_enabled`; add `must_change_password` from the same row, with the same honesty rule: `false` only when the row was actually read. `SetUserMustChangePassword(false)` is called by `ChangePassword` on success.

- [ ] **Step 5: Run the gates**

```bash
gofmt -s -w cmd/server
go test ./cmd/... ./internal/server/... -count=1
go build ./...
```

Expected: PASS.

### Task 8: F-03 and F-08, then commit group 1

**Files:**
- Modify: `internal/auth/service.go` (`SetupTOTP`, `VerifyTOTP`)
- Modify: `internal/server/handlers/auth.go` (the TOTP handlers)
- Modify: `internal/core/invite/service.go` (creation password policy)
- Test: `internal/auth/service_test.go`, `internal/core/invite/service_test.go`

- [ ] **Step 1: Write the failing test**

```go
// TestSetupTOTPStoresNothing: an unverified secret must not reach the database
// (F-03). The pre-SP8a flow wrote totp_secret_encrypted on setup.
func TestSetupTOTPStoresNothing(t *testing.T) {
	svc, q := newTestAuthService(t)
	res, err := svc.SetupTOTP(context.Background(), testUserID)
	require.NoError(t, err)
	require.NotEmpty(t, res.Secret)

	user, err := q.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true})
	require.NoError(t, err)
	require.False(t, user.TotpEnabled)
	require.False(t, user.TotpSecretEncrypted.Valid, "setup must not persist a secret")

	// The first successful verification writes it and enables TOTP.
	require.NoError(t, svc.VerifyTOTP(context.Background(), testUserID, res.Secret, codeFor(t, res.Secret)))
	user, err = q.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true})
	require.NoError(t, err)
	require.True(t, user.TotpEnabled)
	require.True(t, user.TotpSecretEncrypted.Valid)
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/auth/ -run TestSetupTOTPStoresNothing -count=1`
Expected: FAIL — setup currently persists the encrypted secret.

- [ ] **Step 3: Implement the F-03 flow**

Change `SetupTOTP` to generate and return the secret without writing. Change `VerifyTOTP` to take the secret, prove the supplied code is valid for it, and then write the encrypted secret together with `totp_enabled = TRUE` in one statement:

```go
func (s *AuthService) VerifyTOTP(ctx context.Context, userID uuid.UUID, secret, code string) error {
	if !ValidateTOTPCode(secret, code) {
		return ErrInvalidTOTPCode
	}
	encrypted, err := crypto.EncryptSecret(secret)
	if err != nil {
		return err
	}
	return s.querier.EnableTOTPWithSecret(ctx, pgtype.UUID{Bytes: userID, Valid: true}, encrypted)
}
```

```sql
-- name: EnableTOTPWithSecret :exec
UPDATE users
SET totp_secret_encrypted = $2, totp_enabled = TRUE, totp_last_used_step = 0, updated_at = NOW()
WHERE id = $1;
```

Verify and disable each increment `token_version`; setup does not, because it stores nothing.

- [ ] **Step 4: Apply the 8-character policy at creation**

`internal/core/invite/service.go:186` already enforces `MinPasswordLength = 8`; keep it, and add a test that a 7-character password is rejected and an 8-character one is accepted. Do not add a digit or symbol rule. Delete the invented client-side rule in `web/src/pages/SettingsPage.tsx:501` and correct the strings at `:1713` and `:1756` in Task 19, where the frontend is already being touched.

- [ ] **Step 5: Commit group 1**

```bash
TEST_DATABASE_URL= make check
go test -race ./... -count=1
./scripts/coverage.sh --min 50.0 --diff 50.0
diff -r migrations/ internal/db/migrations/
git add internal/auth internal/db internal/core/invite internal/server internal/sse cmd/server web/src
git commit -m "fix(auth): harden sessions, TOTP setup and password policy (SP8a)"
```

Expected: `make check` exits 0 including the live-database script, race is clean, coverage passes, the migrations directories are identical.

---

# Commit Group 2 — RBAC matrix and stable ownership

### Task 9: The permission map

**Files:**
- Create: `internal/auth/rbac.go`
- Create: `internal/auth/rbac_test.go`
- Modify: `internal/server/router.go`

**Interfaces:**
- Produces:

```go
const (
	PermUsersAdmin       Permission = "users.admin"
	PermBackup           Permission = "backup.admin"
	PermSettingsExport   Permission = "settings.export"
	PermEmailTemplates   Permission = "email_templates.admin"
	PermContactsRead     Permission = "contacts.read"
	PermContactsWrite    Permission = "contacts.write"
	PermContactsDelete   Permission = "contacts.delete"
	PermDealsRead        Permission = "deals.read"
	PermDealsWrite       Permission = "deals.write"
	PermDealsDelete      Permission = "deals.delete"
	PermDealsRevoke      Permission = "deals.revoke"
	PermContactsImport   Permission = "contacts.import"
	PermNotesWrite       Permission = "notes.write"
	PermEmailsTeam       Permission = "emails.team"
	PermEmailsSend       Permission = "emails.send"
	PermTelephony        Permission = "telephony.use"
	PermAI               Permission = "ai.use"
	PermAIObservability  Permission = "ai.observability"
	PermReportsView      Permission = "reports.view"
	PermReportsExport    Permission = "reports.export"
	PermAutomationsRead  Permission = "automations.read"
	PermAutomationsWrite Permission = "automations.write"
	PermAutomationsRun   Permission = "automations.run"
	PermOwnOnly          Permission = "own.only"
)

type Permission string

func HasPermission(role string, p Permission) bool
func RequirePermission(p Permission) func(http.Handler) http.Handler
```

- [ ] **Step 1: Write the failing test**

`internal/auth/rbac_test.go` encodes the approved matrix from spec §6.2 as data and asserts `HasPermission` for all four roles:

```go
// TestRoleMatrixMatchesTheApprovedDesign is the machine-checked copy of
// docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md §12.1.
func TestRoleMatrixMatchesTheApprovedDesign(t *testing.T) {
	matrix := []struct {
		perm  Permission
		admin, vertrieb, backoffice, benutzer bool
	}{
		{PermUsersAdmin, true, false, false, false},
		{PermBackup, true, false, false, false},
		{PermSettingsExport, true, false, false, false},
		{PermEmailTemplates, true, false, false, false},
		{PermContactsRead, true, true, true, true},
		{PermContactsWrite, true, true, true, false},
		{PermContactsDelete, true, false, false, false},
		{PermDealsRead, true, true, true, true},
		{PermDealsWrite, true, true, true, false},
		{PermDealsDelete, true, true, false, false},
		{PermDealsRevoke, true, true, true, false},
		{PermContactsImport, true, true, true, false},
		{PermNotesWrite, true, true, true, false},
		{PermEmailsTeam, true, true, true, false},
		{PermEmailsSend, true, true, true, false},
		{PermTelephony, true, true, true, false},
		{PermAI, true, true, true, false},
		{PermAIObservability, true, false, false, false},
		{PermReportsView, true, true, true, false},
		{PermReportsExport, true, false, true, false},
		{PermAutomationsRead, true, true, true, false},
		{PermAutomationsWrite, true, false, false, false},
		{PermAutomationsRun, true, true, false, false},
	}
	for _, row := range matrix {
		require.Equal(t, row.admin, HasPermission("ADMIN", row.perm), "ADMIN/%s", row.perm)
		require.Equal(t, row.vertrieb, HasPermission("VERTRIEB", row.perm), "VERTRIEB/%s", row.perm)
		require.Equal(t, row.backoffice, HasPermission("BACKOFFICE", row.perm), "BACKOFFICE/%s", row.perm)
		require.Equal(t, row.benutzer, HasPermission("BENUTZER", row.perm), "BENUTZER/%s", row.perm)
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/auth/ -run TestRoleMatrix -count=1`
Expected: FAIL to compile — `HasPermission` is undefined.

- [ ] **Step 3: Implement the map**

Define `rolePermissions map[string]map[Permission]bool` from the same table, `HasPermission` reading it (unknown role → false), and `RequirePermission` mirroring `RequireAnyRole` but reading the map. The `OwnOnly` marker is not a permission to check; it is the read-scope signal used in Tasks 12–13.

- [ ] **Step 4: Replace the router's role lists**

Change the twelve `RequireAnyRole(...)` / `RequireRole(...)` call sites in `internal/server/router.go` to `auth.RequirePermission(...)` per the matrix, and add the previously missing gates: `/telephony` (`:528`), `GET /automations` (`:543-544`), `/notifications` (`:551-557`), the `/ai` group (`:483-499`, with `GET /ai/observability` additionally `PermAIObservability`), and `POST /calculator/solar` (`:502-503`).

`GET /api/v1/reports/sales` gains VERTRIEB; `GET /api/v1/export/contacts.csv` gains BACKOFFICE. `internal/server/handlers/users.go:123` and `:203` stay as role validation for request payloads and are not converted.

- [ ] **Step 5: Run the gates**

```bash
gofmt -s -w internal/auth internal/server
go test ./internal/auth/... ./internal/server/... -count=1
```

Expected: PASS.

### Task 10: Fail-closed claim extraction and the ownership check

**Files:**
- Create: `internal/server/handlers/authz.go`
- Create: `internal/server/handlers/authz_test.go`
- Modify: `internal/server/handlers/appointments.go:103,136,170`, `deals.go:189,308`, `notes.go:109,141`, `todos.go:138`, `notifications.go:49`

**Interfaces:**
- Produces:

```go
func claimsFrom(r *http.Request) (*auth.AccessClaims, bool)
func canModify(claims *auth.AccessClaims, owner pgtype.UUID) bool
func canRead(claims *auth.AccessClaims, owner pgtype.UUID) bool
```

- [ ] **Step 1: Write the failing test**

```go
// TestCanModifyNeverGrantsOnANilOwner is the NULL-owner rule: an unowned record
// is ADMIN-only. Defaulting to "everyone may write" is what the free-text check
// did by accident.
func TestCanModifyNeverGrantsOnANilOwner(t *testing.T) {
	sales := &auth.AccessClaims{UserID: uuid.MustParse(testUserID.String()), Role: "VERTRIEB"}
	admin := &auth.AccessClaims{UserID: uuid.MustParse(otherUserID.String()), Role: "ADMIN"}

	nilOwner := pgtype.UUID{}
	require.False(t, canModify(sales, nilOwner), "a NULL owner must not grant write access")
	require.True(t, canModify(admin, nilOwner))

	owned := pgtype.UUID{Bytes: sales.UserID, Valid: true}
	require.True(t, canModify(sales, owned))
	require.True(t, canModify(admin, owned))
	require.False(t, canModify(sales, pgtype.UUID{Bytes: admin.UserID, Valid: true}))
}

// TestClaimsFromIsFailClosed: a claim-less context must not authorize anything.
func TestClaimsFromIsFailClosed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	claims, ok := claimsFrom(req)
	require.False(t, ok)
	require.Nil(t, claims)
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/server/handlers/ -run 'TestCanModify|TestClaimsFrom' -count=1`
Expected: FAIL to compile — the helpers do not exist.

- [ ] **Step 3: Implement the helpers**

```go
// claimsFrom returns the authenticated claims or ok=false. It is the only
// supported way for a handler to read claims: a missing value must deny.
func claimsFrom(r *http.Request) (*auth.AccessClaims, bool) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// canModify applies the ownership rule: ADMIN, or the owner. A NULL owner is
// never a write permission for a non-admin. The owner must be a UUID column;
// free-text columns such as notes.author are not usable here.
func canModify(claims *auth.AccessClaims, owner pgtype.UUID) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "ADMIN" {
		return true
	}
	if !owner.Valid {
		return false
	}
	return uuid.UUID(owner.Bytes) == claims.UserID
}
```

- [ ] **Step 4: Replace the nine fail-open sites**

At each of `appointments.go:103,136,170`, `deals.go:189,308`, `notes.go:109,141`, `todos.go:138`, `notifications.go:49`, replace `if claims != nil && claims.Role != "ADMIN" { ... }` with:

```go
claims, ok := claimsFrom(r)
if !ok {
	writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
	return
}
```

and then the ownership check against the correct UUID column: `deals.assigned_to` and `todos.assigned_to` (existing), `notes.user_id` and `appointments.user_id` (new). Delete the `claims.UserID.String()` comparison against `appointments.assigned_to`, which cannot hold a UUID.

- [ ] **Step 5: Fix the notification null-out**

`internal/core/notification/service.go:81-103` skips the ownership check when `userID == nil`. Change it so a nil user id is an error, not a bypass, and add a test asserting a non-admin cannot mark another user's notification read.

- [ ] **Step 6: Run the gates**

```bash
gofmt -s -w internal/server/handlers internal/core/notification
go test ./internal/server/... ./internal/core/notification/... -count=1
rg -n "claims != nil &&" internal/server/handlers/
```

Expected: PASS, and the last command returns **no matches**.

### Task 11: Ownership on create, update and reassign

**Files:**
- Modify: `internal/server/handlers/contacts.go`, `companies.go`, `notes.go`, `appointments.go`, `emails.go`
- Test: `internal/server/handlers/ownership_test.go`

**Interfaces:**
- Consumes: `canModify`, `claimsFrom` from Task 10.
- Produces: every create path sets the owner server-side; every update/delete path calls `canModify`; reassignment requires ADMIN or the current owner.

- [ ] **Step 1: Write the failing tests**

```go
// TestCreateIgnoresAnOwnerInTheBody: the owner is the creator, full stop. A body
// field that sets the owner lets a caller assign a record to someone else.
func TestCreateIgnoresAnOwnerInTheBody(t *testing.T) {
	h := newOwnershipHandler(t, testUserID, "VERTRIEB")
	rec := performJSON(t, h, http.MethodPost, "/api/v1/contacts", map[string]any{
		"first_name": "Max", "last_name": "Mustermann",
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	contact := decodeContact(t, rec)
	require.Equal(t, testUserID.String(), contact.OwnerUserID.String(),
		"the creator owns the record regardless of the request body")
}

// TestOnlyOwnerOrAdminMayReassign
func TestOnlyOwnerOrAdminMayReassign(t *testing.T) {
	// third party cannot hand a record away
	rec := performJSON(t, otherHandler, http.MethodPut, "/api/v1/contacts/"+contactID, map[string]any{
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusForbidden, rec.Code)

	// the current owner may
	rec = performJSON(t, ownerHandler, http.MethodPut, "/api/v1/contacts/"+contactID, map[string]any{
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 2: Verify they fail**

Run: `go test ./internal/server/handlers/ -run 'TestCreateIgnoresAnOwner|TestOnlyOwnerOrAdmin' -count=1`
Expected: FAIL — the body owner is currently honoured and there is no check.

- [ ] **Step 3: Implement create**

In each create handler, drop `owner_user_id` / `user_id` from the decoded body struct (or ignore it) and set the owner from the claims before calling the service.

- [ ] **Step 4: Implement update and delete**

Before mutating, load the record, call `canModify(claims, owner)`, and return 403 on failure. `appointments` and `notes` must use the new UUID column; the display text columns stay updated from the owner's e-mail so the UI keeps working.

- [ ] **Step 5: Wire the email-account owner**

`email_accounts.owner_user_id` already exists. Set it on `CreateAccount` (`internal/server/handlers/emails.go:163-194`) to the creator, enforce `canModify` on update, delete, send and tag, and re-assign it on update only for ADMIN or the current owner.

- [ ] **Step 6: Run the gates**

```bash
gofmt -s -w internal/server/handlers
go test ./internal/server/handlers/... -count=1
```

Expected: PASS.

### Task 12: Read scoping for BENUTZER

**Files:**
- Modify: `internal/db/queries/contacts.sql`, `companies.sql`, `deals.sql`, `todos.sql`, `notes.sql`, `appointments.sql`, `emails.sql`
- Regenerate: the corresponding `*.sql.go` and `querier.go`
- Modify: `internal/db/demo/querier.go`
- Modify: the list handlers and `internal/core/*/service.go` that call them
- Test: `internal/server/handlers/rbac_routes_test.go`

**Interfaces:**
- Consumes: `auth.HasPermission`, `PermOwnOnly`.
- Produces: every list query takes an optional owner filter; a `BENUTZER` request always passes its own user id.

- [ ] **Step 1: Write the failing test**

```go
// TestBenutzerSeesOnlyAssignedRecords is the row-level half of the matrix.
// A gate that admits BENUTZER is not enough if the list returns everything.
func TestBenutzerSeesOnlyAssignedRecords(t *testing.T) {
	// seed: one contact owned by benutzer, one owned by sales
	rec := performJSON(t, benutzerHandler, http.MethodGet, "/api/v1/contacts", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	contacts := decodeContacts(t, rec)
	require.Len(t, contacts, 1)
	require.Equal(t, benutzerID.String(), contacts[0].OwnerUserID.String())

	// and the detail view of a foreign record is refused
	rec = performJSON(t, benutzerHandler, http.MethodGet, "/api/v1/contacts/"+foreignContactID, nil)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/server/handlers/ -run TestBenutzerSeesOnlyAssigned -count=1`
Expected: FAIL — the list returns both records.

- [ ] **Step 3: Add the owner filter to the queries**

For each list query, add a nullable owner parameter and filter on it. `ListContacts` becomes, in the existing style:

```sql
-- name: ListContacts :many
SELECT * FROM contacts
WHERE (sqlc.narg('owner_user_id')::uuid IS NULL OR owner_user_id = sqlc.narg('owner_user_id')::uuid)
ORDER BY created_at DESC;
```

Regenerate, mirror the filter in the demo querier, and update every caller. Callers pass `pgtype.UUID{}` for roles without `PermOwnOnly` and the caller's own id for `BENUTZER`.

- [ ] **Step 4: Restrict the email list by mailbox**

`BENUTZER` sees only `email_accounts` rows it owns, and messages only from those accounts. Apply the same nullable filter to the account list and thread the account ownership check into the message endpoints.

- [ ] **Step 5: Run the gates**

```bash
/Users/ewenwitte/go/bin/sqlc generate && gofmt -s -w internal/db
go build ./...
go test ./internal/db/... ./internal/server/... -count=1
```

Expected: PASS.

### Task 13: Route × role table test

**Files:**
- Create: `internal/server/rbac_routes_test.go`

**Interfaces:**
- Consumes: the real router, the permission map, the ownership rule.
- Produces: one table asserting, for every route group, the status each role receives.

- [ ] **Step 1: Write the test**

Build the real router with a demo querier seeded with one user per role, then walk the matrix:

```go
// TestRouteMatrixAcrossRoles is the machine-checked enforcement of the approved
// role matrix. It drives the real router, not the handlers in isolation, so a
// middleware that is not actually mounted fails the test.
func TestRouteMatrixAcrossRoles(t *testing.T) {
	cases := []struct {
		method, path string
		want         map[string]int
	}{
		{http.MethodGet, "/api/v1/users", map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/backup/export", map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/reports/sales", map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/export/contacts.csv", map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 200, "BENUTZER": 403}},
		{http.MethodPost, "/api/v1/telephony/calls", map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/automations/", map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 403}},
		{http.MethodPost, "/api/v1/automations", map[string]int{"ADMIN": 201, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/ai/observability", map[string]int{"ADMIN": 200, "VERTRIEB": 403, "BACKOFFICE": 403, "BENUTZER": 403}},
		{http.MethodDelete, "/api/v1/deals/" + dealID, map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 403, "BENUTZER": 403}},
		{http.MethodGet, "/api/v1/notifications/unread", map[string]int{"ADMIN": 200, "VERTRIEB": 200, "BACKOFFICE": 200, "BENUTZER": 200}},
	}
	for _, tc := range cases {
		for role, want := range tc.want {
			t.Run(role+" "+tc.method+" "+tc.path, func(t *testing.T) {
				require.Equal(t, want, doAs(t, role, tc.method, tc.path).Code)
			})
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/server/ -run TestRouteMatrixAcrossRoles -count=1`
Expected: PASS. Any 200 where the matrix says 403 fails, and so does any 403 where it says 200.

### Task 14: Bulk-assign unowned contacts and companies

**Files:**
- Create: `internal/server/handlers/ownership_admin.go`
- Modify: `internal/server/router.go`
- Test: `internal/server/handlers/ownership_admin_test.go`

**Interfaces:**
- Consumes: `ListUnownedContacts`, `ListUnownedCompanies`, `SetContactOwner`, `SetCompanyOwner` from Task 1.
- Produces: `POST /api/v1/ownership/bulk-assign` with body `{"entity":"contacts|companies","owner_user_id":"<uuid>","limit":500}`.

- [ ] **Step 1: Write the failing test**

```go
// TestBulkAssignOnlyTouchesUnownedRows: the action exists so an upgrade does not
// leave the whole CRM admin-only. It must never move a record that already has
// an owner.
func TestBulkAssignOnlyTouchesUnownedRows(t *testing.T) {
	rec := performJSON(t, adminHandler, http.MethodPost, "/api/v1/ownership/bulk-assign", map[string]any{
		"entity": "contacts", "owner_user_id": salesID.String(), "limit": 500,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Assigned int `json:"assigned"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 2, body.Assigned)

	// The already-owned contact kept its owner.
	contact := fetchContact(t, ownedContactID)
	require.Equal(t, otherUserID.String(), contact.OwnerUserID.String())

	// Non-admin is refused.
	rec = performJSON(t, salesHandler, http.MethodPost, "/api/v1/ownership/bulk-assign", map[string]any{
		"entity": "contacts", "owner_user_id": salesID.String(),
	})
	require.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 2: Implement**

Require `PermUsersAdmin` (or a dedicated `PermOwnershipAdmin`), validate the entity against a two-value allowlist, resolve the owner to a real `ACTIVE` user, and stop at `limit` (default 500, cap 5000). Audit the action with the count and the owner. Document the SQL alternative in the spec appendix so an operator with no UI access can do the same thing.

- [ ] **Step 3: Run the gates**

```bash
gofmt -s -w internal/server
go test ./internal/server/... -count=1
```

### Task 15: Live-database ownership test

**Files:**
- Create: `internal/server/handlers/ownership_postgres_test.go`
- Modify: `scripts/test-backup-integration.sh`

- [ ] **Step 1: Write the gated test**

```go
func postgresURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return url
}

// TestOwnershipBackfillAndScopesAgainstRealPostgres proves the migration's
// backfill and the read scopes against a real database. The demo querier cannot
// express either.
func TestOwnershipBackfillAndScopesAgainstRealPostgres(t *testing.T) {
	url := postgresURL(t)
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()

	seeded := seedUser(t, pool, "sp8a-backfill@example.de", "VERTRIEB")
	t.Cleanup(func() { deleteUser(t, pool, seeded) })

	noteID := insertNoteWithAuthor(t, pool, seeded, seeded)  // author = the user's e-mail
	appointmentID := insertAppointmentWithAssignee(t, pool, seeded, seeded)
	orphanNoteID := insertNoteWithAuthor(t, pool, seeded, "Vertriebsmitarbeiter")

	// Apply the migration's backfill statements exactly as written.
	reapplyBackfill(t, pool)

	var noteOwner, apptOwner pgtype.UUID
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT user_id FROM notes WHERE id = $1", noteID).Scan(&noteOwner))
	require.True(t, noteOwner.Valid)
	require.Equal(t, seeded[:], fmt.Sprintf("%x", noteOwner.Bytes))

	require.NoError(t, pool.QueryRow(context.Background(), "SELECT user_id FROM appointments WHERE id = $1", appointmentID).Scan(&apptOwner))
	require.True(t, apptOwner.Valid, "appointments.assigned_to holds an e-mail and must backfill")

	var orphanOwner pgtype.UUID
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT user_id FROM notes WHERE id = $1", orphanNoteID).Scan(&orphanOwner))
	require.False(t, orphanOwner.Valid, "an unmatched author stays NULL, which is ADMIN-only for writes")
}
```

- [ ] **Step 2: Wire it into the live script**

In `scripts/test-backup-integration.sh`, add a second invocation beside the existing backup and invite ones:

```bash
echo "🧪 Running the ownership backfill and scope tests against a real pgvector database"
go test ./internal/server/handlers -run 'TestOwnership.*Postgres' -count=1 -v
```

- [ ] **Step 3: Run the gates**

```bash
./scripts/test-backup-integration.sh
go test ./... -count=1
```

Expected: the script reports the ownership tests passing; without `TEST_DATABASE_URL` they skip cleanly and `go test ./...` is unaffected.

### Task 16: Commit group 2

```bash
gofmt -s -l .
go vet ./...
TEST_DATABASE_URL= make check
go test -race ./... -count=1
./scripts/coverage.sh --min 50.0 --diff 50.0
diff -r migrations/ internal/db/migrations/
rg -n "claims != nil &&" internal/server/handlers/    # must be empty
git add internal/auth internal/db internal/core internal/server
git commit -m "fix(auth): enforce the RBAC role matrix and stable resource ownership (SP8a)"
```

---

# Commit group 3 — Frontend, guards and the live session test

### Task 17: Cross-tab-safe refresh

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `e2e/auth.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
test('a 401 triggers exactly one refresh even with parallel requests', async ({ page }) => {
  const refreshCalls: string[] = [];
  let refreshInFlight = 0;
  let maxConcurrent = 0;

  await page.route('**/api/v1/me', async (route) => {
    if (refreshInFlight > 0) { await route.fulfill({ status: 200, body: meBody }); return; }
    await route.fulfill({ status: 401, body: '{"error":"unauthorized"}' });
  });
  await page.route('**/api/v1/auth/refresh', async (route) => {
    refreshCalls.push(Date.now().toString());
    refreshInFlight++;
    maxConcurrent = Math.max(maxConcurrent, refreshInFlight);
    await new Promise((r) => setTimeout(r, 300));
    refreshInFlight--;
    await route.fulfill({ status: 200, headers: { 'Set-Cookie': 'access_token=new; Path=/; HttpOnly' }, body: '{}' });
  });

  await login(page);
  await Promise.all([
    page.evaluate(() => fetch('/api/v1/contacts', { credentials: 'include' })),
    page.evaluate(() => fetch('/api/v1/deals', { credentials: 'include' })),
    page.evaluate(() => fetch('/api/v1/todos', { credentials: 'include' })),
  ]);

  expect(refreshCalls).toHaveLength(1);
  expect(maxConcurrent).toBe(1);
});
```

- [ ] **Step 2: Verify it fails**

Run: `cd web && pnpm exec playwright test --config=../playwright.config.ts ../e2e/auth.spec.ts --project=chromium -g 'exactly one refresh'`
Expected: FAIL — three parallel 401s currently produce three refreshes.

- [ ] **Step 3: Implement the lock**

In `web/src/api/client.ts`, wrap the refresh call:

```ts
let refreshPromise: Promise<void> | null = null;

async function withRefreshLock<T>(fn: () => Promise<T>): Promise<T> {
  if (!refreshPromise) {
    refreshPromise = fn().finally(() => { refreshPromise = null; });
  }
  return refreshPromise as unknown as T;
}

// Cross-tab coordination: two tabs have separate heaps, so a module promise is
// not enough. The Web Lock makes one tab the refresher; the others wait and
// then reuse the shared cookie jar.
async function withCrossTabLock<T>(fn: () => Promise<T>): Promise<T> {
  const locks = (navigator as Navigator & { locks?: LockManager }).locks;
  if (!locks) return withRefreshLock(fn);
  return locks.request('openlocalcrm-refresh', { mode: 'exclusive' }, () => withRefreshLock(fn));
}
```

Route every 401-triggered refresh through `withCrossTabLock`. Keep the `UNAUTHORIZED_IS_NOT_SESSION_EXPIRY` exclusion for `/api/v1/auth/totp/disable` that SP7 added.

- [ ] **Step 4: Run the web gates**

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck
cd web && pnpm exec playwright test --config=../playwright.config.ts ../e2e/auth.spec.ts --project=chromium
```

Expected: PASS, with the pre-existing `auth` password-policy spec still failing for its own documented reason until Task 19 updates its assertion.

### Task 18: Owner pickers and permission-aware controls

**Files:**
- Modify: `web/src/pages/ContactsPage.tsx`, `CompaniesPage.tsx`, `NotesPage.tsx`, `CalendarPage.tsx`, `SettingsPage.tsx`

- [ ] **Step 1: Implement the picker**

Add a shared `OwnerSelect` component that loads users from the admin-only endpoint when the current role may read them, and otherwise renders the owner as read-only text. The create form never sends an owner: the server sets it. The edit form sends one only when the user is ADMIN or the current owner.

- [ ] **Step 2: Gate the controls**

Derive `canEdit` from the session role and the record's owner, matching the server rule exactly: ADMIN, or the owner; a `NULL` owner means ADMIN only. Hide or disable the edit and delete controls otherwise, with a short German reason. The frontend is a convenience here — the server is what enforces the rule — so a stale UI must fail closed rather than offer an action that 403s.

- [ ] **Step 3: Gates**

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck
cd web && pnpm build
```

### Task 19: Password nag, corrected policy text, and route guards (F-45)

**Files:**
- Modify: `web/src/context/AuthContext.tsx`, `web/src/pages/SettingsPage.tsx`, `web/src/pages/LoginPage.tsx`, `web/src/App.tsx`
- Test: `e2e/auth.spec.ts`

- [ ] **Step 1: Correct the policy text**

The server enforces a minimum of 8 characters and nothing else. Change `SettingsPage.tsx:501` to drop the invented digit-and-symbol check, and `:1713` and `:1756` to state the real rule. Update the matching assertion in `e2e/auth.spec.ts`. A UI that claims a rule the server does not enforce is the same defect class this project has been removing.

- [ ] **Step 2: Add the `must_change_password` banner**

Add `must_change_password?: boolean | null` to `UserSession`, surface a non-dismissable banner in the app shell while it is true, and clear it only after a successful password change followed by a `/me` re-read. `null` renders as unknown, never as "no change needed".

- [ ] **Step 3: Align the route guards (F-45)**

`web/src/App.tsx` must derive the protected-route decision from the session state alone: render nothing protected until the session is resolved, redirect on an explicit unauthenticated result, and never fall back to a default that looks authenticated. Verify the unauthenticated login screen still renders, since `/api/v1/health` stays open and the login page depends on it.

- [ ] **Step 4: Gates**

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck
cd web && pnpm exec playwright test --config=../playwright.config.ts ../e2e/auth.spec.ts --project=chromium
```

### Task 20: Live session test

**Files:**
- Create: `internal/auth/session_postgres_test.go`
- Modify: `scripts/test-backup-integration.sh`

- [ ] **Step 1: Write the gated test**

```go
// TestRefreshRotationAgainstRealPostgres proves the rotation transaction, the
// replay path and the version bump against a real database. The demo querier
// runs everything under one mutex and cannot express any of it.
func TestRefreshRotationAgainstRealPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()

	userID := seedSessionUser(t, pool)
	t.Cleanup(func() { deleteUser(t, pool, userID) })

	first := insertRefreshToken(t, pool, userID, uuid.New())
	consumed, err := consume(t, pool, first.hash)
	require.NoError(t, err)
	require.Equal(t, first.familyID, consumed.FamilyID)

	// A second consume of the same hash must return no row: that is the replay
	// signal, and it is what the service turns into a family revocation.
	_, err = consume(t, pool, first.hash)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	// A concurrent consume yields exactly one winner.
	winner := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := consume(t, pool, first.hash)
			winner <- err
		}()
	}
	var successes int
	for i := 0; i < 2; i++ {
		if err := <-winner; err == nil { successes++ }
	}
	require.Equal(t, 1, successes, "a refresh token must be consumable exactly once")

	// Incrementing the version invalidates every issued access token.
	before := currentVersion(t, pool, userID)
	_, err = pool.Exec(context.Background(), "UPDATE users SET token_version = token_version + 1 WHERE id = $1", userID)
	require.NoError(t, err)
	require.Greater(t, currentVersion(t, pool, userID), before)
}
```

- [ ] **Step 2: Wire it into the script**

```bash
echo "🧪 Running the session rotation tests against a real pgvector database"
go test ./internal/auth -run 'TestRefreshRotation.*Postgres' -count=1 -v
```

- [ ] **Step 3: Verify the mutation**

Remove the `used_at IS NULL` predicate from `ConsumeRefreshToken` and confirm the "consumable exactly once" assertion fails; remove the `IncrementUserTokenVersion` call from the replay path and confirm the pre-replay access token is still accepted, which is what fails the test. Restore both and re-run green.

### Task 21: E2E updates for the new behaviour

**Files:**
- Modify: `e2e/auth.spec.ts`, `e2e/inbox.spec.ts`, `e2e/deals-kanban.spec.ts`, `e2e/calendar-and-telephony.spec.ts`, `e2e/automations.spec.ts`

**Interfaces:**
- Consumes: the matrix from Task 9, ownership from Task 11, read scoping from Task 12.

- [ ] **Step 1: Update the specs that assume full visibility**

A `BENUTZER` session in the demo data now sees only its own records. Every spec that logs in as `BENUTZER` and expects a shared list must be updated to use the `ADMIN` session, or to assert the scoped result. Search for `BENUTZER` in `e2e/` and fix each occurrence deliberately rather than blanket-replacing.

- [ ] **Step 2: Add the new specs**

- `logout-all` signs out on a second session as well.
- the `must_change_password` banner appears for the bootstrap admin and disappears after the change.
- a `BENUTZER` session cannot see a foreign record in the contacts list, and the edit control is absent.

- [ ] **Step 3: Run both suites**

```bash
cd web && pnpm exec playwright test --config=../playwright.config.ts ../e2e/auth.spec.ts ../e2e/inbox.spec.ts ../e2e/deals-kanban.spec.ts --project=chromium
cd web && pnpm exec playwright test --config=../playwright.config.ts --project=chromium
```

Expected: the focused specs pass; the full suite reports the same 11 pre-existing failures and no new one. Report any difference explicitly.

### Task 22: Full SP8a verification gate

**Files:** none.

- [ ] **Step 1: Run every gate**

```bash
cd web && pnpm format && pnpm format:check && pnpm lint && pnpm typecheck && pnpm build && cd ..
TEST_DATABASE_URL= make check
go test -race ./... -count=1
./scripts/coverage.sh --min 50.0 --diff 50.0
diff -r migrations/ internal/db/migrations/
test -L migrations
git status --porcelain
```

Expected: all exit 0; the migrations directories are identical; the symlink is intact; the tree is clean after the commit.

- [ ] **Step 2: Verify the three-commit contract**

```bash
SP8A_BASE=$(git merge-base main HEAD)
git log --oneline "$SP8A_BASE"..HEAD
```

Expected: exactly the three SP8a commits, with no SP1–SP7 rewrite.

- [ ] **Step 3: Confirm the release notes**

`docs/API_REFERENCE.md` and `CHANGELOG.md` must record the two breaking changes from spec §11: pre-upgrade access tokens are rejected, and `/auth/refresh` is cookie-only. Also document `POST /api/v1/auth/logout-all` and `POST /api/v1/ownership/bulk-assign`.

### Task 23: Commit group 3

```bash
git add internal/auth internal/server internal/db web/src e2e scripts docs/API_REFERENCE.md CHANGELOG.md
git commit -m "feat(ui): wire ownership, password nag and route guards with the live session test (SP8a)"
git log --oneline $(git merge-base main HEAD)..HEAD
```

Expected: exactly three SP8a commits, working tree clean, nothing pushed.

---

## Plan Self-Review

**Spec coverage.** §2 migration → Task 1. §3.1 required claim → Task 2. §3.1 middleware and SSE → Task 3. §3.2 rotation, transaction, family revocation, bump → Task 4. §3.3 cross-tab → Task 17. §3.4 logout and the seven bump triggers → Tasks 3, 5, 7, 8. §3.5 `RevokeUser` and the 60-second backstop → Task 5. §4 no tokens in JSON → Task 6. §5 bootstrap and the flag → Task 7 and Task 19. §6.1 permission map → Task 9. §6.2 matrix → Tasks 9, 13. §6.3 fail-closed and the owner-column table → Task 10. §6.4 ownership rule → Task 11. §6.2 read scope → Task 12. §7 F-03 → Task 8, F-08 → Task 8 and Task 19, F-45 → Task 19. §8 backup contract → Task 1, live tests → Tasks 15, 20. §9 error codes → Tasks 6, 10. §10 verification → every task plus 13, 15, 20, 22. §11 release notes → Task 22. §12 residual risks → accepted in the spec; R5 is mitigated by Task 14. §14 commits → the commit map. No spec requirement is unassigned.

**Placeholder scan.** No TBD, no "similar to Task N", no unspecified error handling. Every code step shows the code. Test helpers referenced across tasks — `newTestAuthService`, `seedTestUser`, `failingQuerier`, `setUserStatus`, `countRefreshTokens`, `refreshParams`, `serveWithMiddleware`, `newOwnershipHandler`, `doAs`, `performJSON`, `decodeContact`, `decodeContacts` — are written as helpers in the test file of the task that first uses them, and that step says so.

**Type consistency.** `TokenVersion *int32` is used identically in `jwt.go`, `middleware.go`, Task 4's `issueAccessToken`, and the Task 4 middleware test. `ConsumeRefreshToken` returns `family_id` as `pgtype.UUID` in Task 1 and is read as `consumed.FamilyID` in Task 4. `db.CreateRefreshTokenParams` uses `UserAgent`/`IpAddress` of type `pgtype.Text` and gains `FamilyID` in Task 1; Task 4 constructs it with exactly those fields. `canModify(claims, pgtype.UUID)` has one signature across Tasks 10, 11 and the frontend contract in Task 18. `RequirePermission` is introduced in Task 9 and used only from Task 9 onward. `auth.SessionRevoker` is declared in Task 4 and implemented by `*sse.Hub` in Task 5, with `TestHubSatisfiesTheAuthRevokerSeam` pinning the two together. `TxRunner` is declared in both `internal/db` and `internal/db/demo` with the identical method set `RunTx(ctx, func(db.Querier) error) error`; `*db.Queries` implements it with a real PostgreSQL transaction, `demo.InMemoryQuerier` with a shadow clone that never re-enters its own `sync.RWMutex`. The refresh path and the replay path both go through `(*AuthService).runTx`, never through `(*db.Queries).Begin`, so demo mode exercises the same all-or-nothing semantics as production.
