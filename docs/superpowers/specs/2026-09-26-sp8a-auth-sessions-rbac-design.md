# Sub-Project 8a (SP8a): Authentication, Sessions & RBAC Design Spec

**Status:** Approved
**Target Branch:** `fix/sp8a-auth-sessions-rbac`
**Baseline:** `main` at `e2762fb33df0bd8aed459c3839003e33d5cbb5d6`
**Parent Design:** `docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md` §12 (SP8a) and §12.1 (Defined RBAC Role Matrix)
**Parent Plan:** `docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md` Phase 8, Task 8.1
**Findings Addressed:** F-03, F-06, F-07, F-08, F-09, F-10, F-11, F-14, F-34, F-43, F-45

---

## 1. Purpose and Boundaries

SP8a makes the authentication and session layer durably correct and turns the parent design's declared RBAC matrix into enforced code. Today the session layer cannot be revoked across a restart, refresh-token reuse is answered with a fresh token pair, nine ownership checks are fail-open, six route groups are ungated, ownership is compared against free-text e-mail strings, and the bootstrap admin password is printed to stdout.

The parent design is binding. This document records the concrete SP8a architecture, the two places where a deliberate, recorded decision departs from it, the data flow, failure behavior, and the verification contract.

SP8a does not modify SP1 through SP7. The next migration is `00020`; SP8a adds exactly one migration. The 1024-dimensional embedding contract and every applied migration are untouched.

### 1.1 Deliberate deviations from the parent design

| Parent design | SP8a decision | Rationale |
| --- | --- | --- |
| "Revocation is persisted in PostgreSQL (`revoked_tokens` table)" | `users.token_version`, compared against the database on every authenticated request | One indexed read replaces a per-token row, a sweeper, and an unbounded table. Role, status and password changes invalidate every token at once with a single `UPDATE`. Decided with the user. |
| `must_change_password` "forces immediate password reset upon first login" | The flag is reported on `/me` and surfaced as a UI banner; the API is **not** gated | Decided with the user. The log-line removal and the flag are the security-relevant halves; the hard gate was declined. See §12 residual risk R4. |

## 2. Data Migration

One migration, `internal/db/migrations/00020_sp8a_sessions_and_ownership.sql`:

```sql
ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE refresh_tokens ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ;
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);

ALTER TABLE contacts   ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE companies  ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE notes      ADD COLUMN user_id       UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE appointments ADD COLUMN user_id     UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_contacts_owner   ON contacts(owner_user_id);
CREATE INDEX idx_companies_owner  ON companies(owner_user_id);
CREATE INDEX idx_notes_user       ON notes(user_id);
CREATE INDEX idx_appointments_user ON appointments(user_id);
```

`email_accounts.owner_user_id` already exists (`00013_email_accounts_and_tags.sql`) and is wired rather than added.

### 2.1 Ownership backfill

`notes.author` and `appointments.assigned_to` are free-text columns that hold the author's or assignee's e-mail. The backfill maps them to a user id by case-insensitive e-mail match:

```sql
UPDATE notes SET user_id = u.id
  FROM users u WHERE lower(notes.author) = lower(u.email) AND notes.user_id IS NULL;

UPDATE appointments SET user_id = u.id
  FROM users u WHERE lower(appointments.assigned_to) = lower(u.email)
  AND appointments.user_id IS NULL;
```

Rows whose text does not match any user — including the `"Vertriebsmitarbeiter"` fallback written by `internal/core/note/service.go:154` — stay `NULL`. `contacts` and `companies` have no owner source at all, so all existing rows stay `NULL`. `deals.assigned_to` and `todos.assigned_to` are already `UUID` and need no backfill.

## 3. Session Architecture

### 3.1 Access tokens

Access tokens keep `user_id`, `email`, `role`, the 30-minute lifetime, and Ed25519 signing. One claim is added: `token_version`, mirroring `users.token_version` at issue time.

`token_version` is a **required** claim. A token that lacks it is rejected with 401 rather than defaulting to `0`; defaulting would let every pre-upgrade token pass the check until it expired. This deliberately invalidates every token issued before the upgrade and is a release-note item (§11).

`AuthMiddleware` (`internal/auth/middleware.go:29`) takes an additional dependency:

```go
type TokenVersionVerifier interface {
    CurrentTokenVersion(ctx context.Context, userID pgtype.UUID) (int32, error)
}
```

`*db.Queries` and `demo.InMemoryQuerier` both implement it, so demo mode is unaffected. On a mismatch the request is 401. A database error during the lookup also fails closed with 401 and a log line, because every authorization decision in §6 depends on this read; the alternative — serving the request on stale knowledge — would silently disable the matrix. The process-local `revokedAccessTokens sync.Map` (`internal/auth/service.go:89`) and the 10-second grace cache (`internal/auth/service.go:71-82`) are deleted.

The hand-rolled SSE authentication at `internal/server/router.go:130-156` runs the same version check once at connect time.

### 3.2 Refresh tokens and family revocation

`refresh_tokens` gains `family_id` and `used_at`. Rotation is one transaction: mark the presented token used, insert its successor, and only then return the new pair. Both writes must commit together — if the `used_at` update committed while the insert failed, the client's next legitimate attempt would present an already-used token and be reported as theft. An abort therefore leaves the original token usable and produces no false replay signal.

1. The presented token hash does not exist, or exists but is already past `expires_at` → 401, and **no** family action. An expired token is treated as unknown; revoking a family because an old token was replayed would punish a user for a stale cookie.
2. It exists and `used_at IS NULL` → in one transaction, set `used_at = NOW()` and insert the successor in the same family, then return the new pair. The new row records the current request's `user_agent` and `ip_address`, as today.
3. It exists and `used_at IS NOT NULL` → this is a replay, and it is treated as a compromise of the account rather than of one session. In one transaction SP8a revokes every token in that family **and** increments `users.token_version`, which invalidates every access token already issued to that user, including the legitimate session. The response is 401 and `sse.Hub.RevokeUser` closes that user's open streams. Revoking only the refresh family would leave the stolen access token valid for up to 30 minutes, which is precisely the window the finding is about.

The grace cache is deleted rather than shortened: an unconditional replay window is the finding. Cross-tab coordination (§3.3) reduces false positives; the `used_at` check plus the version bump remains the control. The cost of a false positive is that the user is signed out everywhere and must log in again; that is the correct failure direction for a suspected theft.

The rejection is audited with IP and user agent. When they differ from the fingerprint recorded on the used token, the audit reason distinguishes the two, so a suspected theft stays distinguishable from a lost device. Fingerprint mismatch alone never revokes a family — a mobile client changes address routinely.

`RefreshToken` continues to refuse any user whose status is not `ACTIVE` (`internal/auth/service.go:297`).

### 3.3 Cross-tab coordination

Two browser tabs have separate heaps, so a module-level promise does not stop them racing. Refresh therefore takes a Web Lock when available, falling back to a single in-flight promise plus a `BroadcastChannel` notification:

```ts
if (navigator.locks) {
  return navigator.locks.request('openlocalcrm-refresh', doRefresh);
}
```

The cookie jar is shared, so the tab that wins the lock satisfies the waiters through `Set-Cookie`. A user whose family is revoked by a genuine theft is logged out on every device; that is the intended outcome, not a failure mode to paper over.

### 3.4 Logout and logout-everywhere

`POST /auth/logout` deletes the presented refresh token. With the deny list gone it can no longer invalidate the access token, which remains valid for up to 30 minutes (§12, R3).

`POST /auth/logout-all` is added. It bumps `users.token_version` and deletes every refresh token for the user, closing all sessions on all devices, and calls `sse.Hub.RevokeUser` (§3.5). This is the remedy for a compromised account.

`token_version` is bumped by exactly these operations, each of which also calls `sse.Hub.RevokeUser`:

| Trigger | Location |
| --- | --- |
| Role change | `internal/server/handlers/users.go` `UpdateRole` |
| Status change | `internal/server/handlers/users.go` `UpdateStatus` |
| Password change | `internal/server/handlers/auth.go` `ChangePassword` |
| TOTP enabled (first successful verification) | `internal/server/handlers/auth.go` `VerifyTOTP` |
| TOTP disabled | `internal/server/handlers/auth.go` `DisableTOTP` |
| Logout-everywhere | new handler |
| Refresh-token replay detected | `internal/auth` rotation path (§3.2, case 3) |

`SetupTOTP` is deliberately **not** in this list. Under F-03 (§7) it stores nothing, so there is no security state to invalidate and a bump there would sign the user out of their own session for beginning a setup they have not finished.

### 3.5 Server-Sent Events

`internal/sse/hub.go` stores the subscribing user's id with each connection so `RevokeUser(userID)` can close exactly that user's streams, immediately terminating a long-lived stream after a demotion. A 60-second re-check of `token_version` inside the stream loop is the backstop for changes made outside this process.

Only the keying and the revocation are in scope. Per-user *delivery* of notifications is F-41 and stays in SP8b. The re-check is a per-connection 60-second ticker, so the cost is one timer per open stream, which a single-tenant deployment absorbs.

## 4. Response Bodies Carry No Tokens

`LoginResponse` and the refresh response stop including `token` and `refresh_token` (§F-43). The SPA never read those fields; the HttpOnly cookies already carry both tokens.

The refresh endpoint stops accepting a body token as well. A client that authenticated by reading the JSON body would receive no new token and could not continue, so the cookie is the single mechanism. This is a breaking change for any external consumer and is a release-note item.

## 5. Bootstrap Credentials

`cmd/server/main.go:78-87` stops logging the generated password. When `INITIAL_ADMIN_PASSWORD` is empty, the server generates 32 random bytes as today, writes them to `<STORAGE_PATH>/bootstrap-admin.txt` with mode `0600`, and logs only that path plus a reminder to change the password. The file is deleted when the password first changes.

The bootstrapped admin is created with `must_change_password = TRUE`. `/me` reports the flag; the SPA shows a banner until it clears. `ChangePassword` clears it. Invite acceptance and admin-created users set it to `FALSE`, because those flows set a password the user chose through a validated form.

## 6. RBAC

### 6.1 Single source of truth

`internal/auth/rbac.go` defines named permissions and a role→permission map. The router references permissions; the role strings that are currently duplicated across `internal/server/handlers/users.go:123`, `:203`, twelve inline router calls, and `web/src/context/AuthContext.tsx:7` collapse to this one place.

The four roles remain `ADMIN`, `VERTRIEB`, `BACKOFFICE`, `BENUTZER` (`00001`, extended by `00011`).

### 6.2 The enforced matrix

Taken verbatim from the parent design §12.1:

| Route Group / Domain | ADMIN | VERTRIEB | BACKOFFICE | BENUTZER |
|---|---|---|---|---|
| Contacts / Companies | Full CRUD | Full CRUD | Full CRUD | Read-only + own assigned |
| Deals / Kanban | Full CRUD | Full CRUD | Edit/Update (no delete) | Read-only (assigned) |
| Notes / Todos / Appointments | Full CRUD | Full on assigned/team | Full on assigned/team | Own items only |
| Emails (Inbox, Send, Sync) | Full (team + personal) | Team + personal | Team + personal | Personal mailbox only |
| Telephony & AI Copilot | Full | Full | Full | None |
| Reports & CSV Export | Full | View reports | View reports + CSV export | None |
| Settings / Backup / Connectors / Users / Audit | Full | None | None | None |
| Automations (config, runs, test-run, approve) | Full | Runs: view + approve, test-run | Runs: view | None |
| Notifications | Own | Own | Own | Own |

Two readings are pinned so implementation cannot drift:

- **"assigned/team"** for VERTRIEB and BACKOFFICE resolves to *all* team records. The schema has no team entity, so the phrase carries no per-user restriction; only BENUTZER is restricted to its own items.
- **"Personal mailbox only"** means BENUTZER sees only `email_accounts` rows whose `owner_user_id` is its own user id, and within those, their messages. VERTRIEB, BACKOFFICE and ADMIN see all accounts.

Row filtering is part of the matrix, not an optional extra: `BENUTZER` list endpoints apply an owner filter, and a detail request for an out-of-scope record is 403. An ownership denial returns **403, not 404** — read access is already role-scoped, so a 404 would add a second code path without hiding anything.

`GET /api/v1/reports/sales` currently allows `ADMIN` and `BACKOFFICE`; the matrix adds `VERTRIEB` as "view reports". `GET /api/v1/export/contacts.csv` is currently `ADMIN` only; the matrix allows `ADMIN`, and CSV export for `BACKOFFICE`. `POST /api/v1/deals/{id}/revoke` keeps `ADMIN`, `VERTRIEB`, `BACKOFFICE`. `POST /api/v1/import/contacts` keeps `ADMIN`, `VERTRIEB`, `BACKOFFICE`.

### 6.3 Fail-closed extraction

`internal/server/handlers/authz.go` adds:

```go
func requireClaims(w http.ResponseWriter, r *http.Request) (*auth.AccessClaims, bool)
func canModify(claims *auth.AccessClaims, owner pgtype.UUID) bool
```

`requireClaims` writes 401 and returns `false` when the context holds no claims. It replaces the fail-open `if claims != nil && ...` pattern at `appointments.go:103,136,170`, `deals.go:189,308`, `notes.go:109,141`, `todos.go:138`, and `notifications.go:49`.

`canModify` takes the owner as a `pgtype.UUID` and the caller supplies the correct column. It never parses or compares a free-text column, which is what makes `appointments.go:109` unsafe today: the column holds an e-mail, so the `claims.UserID.String()` comparison against it is meaningless. The column per entity is fixed:

| Entity | Authoritative owner column | Source |
| --- | --- | --- |
| Deals | `deals.assigned_to UUID` | existing (`00002_core_crm_schema.sql:51`) — no new column |
| Todos | `todos.assigned_to UUID` | existing (`00002_core_crm_schema.sql:79`) — no new column |
| Email accounts | `email_accounts.owner_user_id UUID` | existing (`00013`) — wired, not added |
| Contacts | `contacts.owner_user_id UUID` | new in `00020` |
| Companies | `companies.owner_user_id UUID` | new in `00020` |
| Notes | `notes.user_id UUID` | new in `00020`; `notes.author VARCHAR` stays display-only |
| Appointments | `appointments.user_id UUID` | new in `00020`; `appointments.assigned_to VARCHAR(255)` stays display-only and is **never** an authorization input |

Deals and todos therefore keep their existing "assigned-only" rule and their existing column; the change is that the check becomes fail-closed and consistent with everything else. The SP7 review noted that the appointments check compares against a column that cannot hold a UUID — the new `user_id` removes that class of bug instead of working around it.

`internal/core/notification/service.go:81-103` stops treating a nil user id as "skip the ownership check"; a null-out for admins is what let any admin mark any notification read.

`internal/server/handlers/users.go:123` and `:203` remain role validation for the invite and role-change payloads. They are not folded into route permissions.

### 6.4 Ownership rule

Applies to contacts, companies, notes, appointments, deals, todos, and email accounts.

| Case | Rule |
| --- | --- |
| Create | The server sets the owner to the creator. Any owner field in the request body is ignored. |
| Update / delete | Allowed when the caller is `ADMIN`, or when the owner is set and equals the caller. |
| Reassign | Only `ADMIN` or the current owner may change the owner. |
| Owner is `NULL` | Write access is `ADMIN` only. A `NULL` owner is never a write permission for any other role. |

`NULL`-owner rows are therefore a deliberate operational cost: after the upgrade, every existing contact and company is `ADMIN`-only for writes until an owner is assigned. To make that tractable, SP8a adds an admin bulk-assign action over unowned contacts and companies, with an optional owner and a filter, plus a documented SQL alternative. The action is audited.

Read scope follows §6.2, not this table: `BENUTZER` sees a `NULL`-owner record from no list, because `NULL` means assigned to nobody.

## 7. Additional Findings in Scope

**F-03 (TOTP setup race).** `POST /auth/totp/setup` stops writing `totp_secret_encrypted`. It returns the secret for display only. The first successful `POST /auth/totp/verify` takes the secret together with the code, proves the code is valid for that secret, then writes the secret and sets `totp_enabled = TRUE` in one statement. An unverified secret never reaches the database.

**F-08 (password policy at creation).** The policy is a minimum of 8 characters, applied wherever a password is chosen: invite acceptance (`internal/core/invite`) and admin-created users. The backend already enforces exactly this and nothing more — `internal/auth/service.go:431` is the whole of `ChangePassword`'s validation — so F-08 stays at 8 characters and no digit or symbol rule is introduced here.

The frontend currently claims a stricter policy than the server enforces: "Mindestens 8 Zeichen, mindestens 1 Zahl und 1 Sonderzeichen" (`web/src/pages/SettingsPage.tsx:1713`, the placeholder at `:1756`, and the client-side check at `:501`). SP8a corrects those strings to the policy the server actually applies. Claiming a rule the server does not enforce is the same class of defect this project has been removing, and the e2e assertion that matches the old text is updated with it.

**F-45 (frontend route guards).** The SPA guard derives strictly from the authenticated session state, with no optimistic render of a protected page, and is consistent with `/me` returning `null` for unknown TOTP state.

## 8. Backup, Restore and Drill Integration

Every new column is added to the SP7 backup contract, or a restore would silently drop it:

- `internal/db/queries/backup.sql`: `ExportUsersRows`/`UpsertUsersRow`, `ExportRefreshTokensRows`/`UpsertRefreshTokensRow`, and the `contacts`, `companies`, `notes`, `appointments` pairs.
- `internal/core/backup/tables.go`: the per-table column allow-lists, which the restore validator enforces.
- `internal/db/queries/backup.sql`: the new columns in the `TruncateBackupTables` set is unaffected, since no table is added.
- `internal/db/demo/querier.go`: the new methods and the new fields.

The SP7 invariants get explicit regression coverage: the ownership columns round-trip through export and restore, and replace mode still requires the complete 24-table envelope.

The `TEST_DATABASE_URL`-gated live test added by SP8a runs inside `scripts/test-backup-integration.sh`, which is already the canonical live-database gate and already refuses a database it did not provision unless the operator opts in. The disposable fixture applies `00020` automatically; the test asserts the backfill assigned the expected user ids, that unmatched rows stayed `NULL`, and that rotation, replay, and family revocation behave as specified against a real database.

## 9. Error Handling

| Condition | Status | Body |
| --- | --- | --- |
| No, malformed, expired or version-mismatched token | 401 | existing unauthorized error shape |
| Token without a `token_version` claim | 401 | same shape |
| Missing claims in a handler context | 401 | same shape |
| Role not permitted | 403 | existing forbidden shape |
| Ownership or read scope denies | 403 | same shape |

Handlers keep the existing `writeJSONError` pattern. Centralized error sanitization is F-27 and stays in SP8b.

## 10. Verification Contract

1. Route × role table test asserting §6.2 through the real router, including `BENUTZER` list filtering and out-of-scope detail denial.
2. Unit tests: rotation and family revocation, replay rejection, missing-claim rejection, version mismatch, `canModify` for every ownership case, `requireClaims` fail-closed, TOTP setup race, creation password policy, bootstrap file permissions and deletion.
3. Live-database test: backfill correctness, atomic rotation, replay revocation of a whole family, ownership filters.
4. The two highest-consequence guarantees are mutation-verified: removing the `used_at` check must make a replayed token succeed, and making a `NULL` owner writable by a non-admin must fail the test. A third mutation removes the `token_version` bump from the replay path. The test asserts that the access token issued before the replay is rejected with 401; with the bump removed that token is still accepted, and surviving is exactly what fails the test — the mutation is proven by the token outliving the theft, not by the test passing.
5. SP7 regression coverage for the backup round-trip and replace-mode completeness (§8).
6. `make check`, which includes the live-database script; `go test -race ./...`; coverage ≥ 50 % total and ≥ 50 % on changed lines; `migrations` symlink identical to `internal/db/migrations`; `pnpm format`, `pnpm lint`, `pnpm typecheck`, `pnpm build`.
7. E2E: the `must_change_password` banner, owner pickers, `BENUTZER` visibility, logout-everywhere, and the existing SP7 invite and backup specs.

## 11. Release Notes

Two intentional breaking changes:

- Every access token issued before the upgrade is rejected, because `token_version` is required.
- `/auth/refresh` no longer returns a token and no longer accepts a body token; clients must use the cookie.

## 12. Residual Risks

| # | Risk | Why it is accepted |
| --- | --- | --- |
| R1 | Ownership is compared per request against a column read inside the transaction; a row reassigned concurrently is governed by normal isolation. | Accepted; PostgreSQL row locks already serialise the write. |
| R2 | One extra indexed read per authenticated request. | Accepted; cheaper than a persistent deny list and it makes revocation immediate. |
| R3 | After `logout` alone, a stolen access token stays valid up to 30 minutes. | Accepted; `logout-all` is the remedy, and the window is the access-token lifetime. |
| R4 | `must_change_password` is not API-gated, so a bootstrap admin can ignore the banner. | User decision; the log-line removal and the flag are the security-relevant parts. |
| R5 | `BENUTZER` loses visibility of `NULL`-owner records after the upgrade. | Consequence of the matrix; mitigated by the bulk-assign action. |
| R6 | A genuine cross-device race that escapes Web Locks support revokes the family and logs the user out everywhere. | Failing closed on a suspected theft is the intended behaviour. |

## 13. Out of Scope

F-16/F-18/F-19 (CSRF session binding), F-22/F-23 (webhook rate limiting), F-24/F-25 (upload MIME), F-26 (CSV import), F-27 (error leakage), F-28/F-29 (SSRF), F-30 (updater integrity), F-37/F-38/F-39 (transport and cookies), F-40 (health), F-41 (SSE per-user delivery), F-42 (local binding) — all SP8b. The audit hash chain, F-46/F-48, and AI observability — SP8c. P2 documentation, Vitest, and dead-endpoint cleanup — SP9.

## 14. Commits

Three reviewable commits, each independently revertible:

1. `fix(auth): harden sessions, TOTP setup and password policy (SP8a)`
2. `fix(auth): enforce the RBAC role matrix and stable resource ownership (SP8a)`
3. `feat(ui): wire ownership, password nag and route guards with the live session test (SP8a)`
