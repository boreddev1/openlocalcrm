# Round-4 Full Remediation — Program Design

> **Date:** 2026-09-21
> **Input:** `docs/REVIEW_2026-09-21_round4.md` (Round-4 review at HEAD `f890797`, v1.0.6)
> **Scope decision:** Full program — all P0 (fake/simulated features), P1 (security), and
> P2 (feature gaps, tests, docs) findings, decomposed into 9 sequential sub-projects.
> **Strategy decision:** Build everything real — no feature keeps a fabricated success path.
> The only permitted simulation is the explicitly labeled demo mode (`DEMO_MODE=true`).
> **Status:** Approved in brainstorming session 2026-09-21. Implementation plan:
> `docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md`.

---

## 1. Context

Round 4 found that the "fake success" defect is systemic: nearly every subsystem that
depends on an external protocol (email, telephony, calendar push, geocoding, embeddings)
reports success while performing no external work, and round 3's CHANGELOG contains
claims that are demonstrably unfixed. On top of this sit one Critical security finding
(F-31 settings-export credential leak, chainable via the F-15 CSRF bypass), several High
findings (F-20/F-21 rate-limiting model, F-01/F-02 TOTP), and ~30 Medium findings.

This design defines the target architecture for remediating all of it, organized so that
each sub-project is independently implementable, reviewable, and testable.

## 2. Guiding decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Scope | Full program: P0 + P1 + P2, per the review's remediation priority |
| Fake features | **Build real implementations** for E-Mail (SMTP/IMAP), geocoding, pgvector embeddings. **Remove** fake Graph/Exchange/M365 calendar push in favor of clean CRM calendar + RFC 5545 ICS export/invites |
| Email account model | **Both** team mailboxes (admin-managed) and personal mailboxes (per user); **full real SMTP/IMAP** with password/app-password auth; real send & triage |
| Calendar external sync | **Drop Graph/Exchange OAuth sync.** Calendar remains fully functional locally with honest RFC 5545 `.ics` export/download |
| AI Backend requirement | **Mandatory real AI backend** (Ollama or configured cloud provider like OpenAI). No simulated/canned text anywhere; fails honestly (HTTP 502/503) if unreachable; demo mode also requires real AI |
| Geocoding | **Offline PLZ centroids** (bundled open-data table) as default; **optional external provider** via config for address-level precision |
| Memory budget | Steady-state runtime stays under the AGENTS.md **< 420 MB target** (idle/normal-load `crm-server`). **2 GB is a bounded, transient ceiling** for one-off/burst operations only — HNSW index build during the embedding migration, backup/restore streaming, and up to 4 concurrent Argon2id derivations — never a new resting baseline. Resolved 2026-09-21: this reconciles the design with the AGENTS.md invariant instead of overriding it. |
| TOTP / 2FA | **Optional for now** — core authentication relies on password + rate limiting; 2FA is purely opt-in and does not block onboarding or the primary security fixes |
| Program structure | Sequential sub-projects per review priority (Approach A), one branch/PR each |

## 3. Program structure

Nine sub-projects (SP), each: branch `fix/spN-<topic>` → implementation → `make check`
green → coverage gate (≥ 50% diff) → PR against `main`. Order follows the review's
remediation priority (security first, then by fake-success surface size):

| # | Sub-project | Findings |
|---|---|---|
| SP1 | Security Critical/High | F-31, F-15, F-20, F-21, F-32, F-33, F-04, F-05 (TOTP F-01, F-02 optional) |
| SP2 | Real email (SMTP/IMAP) | E1–E8, F-36 |
| SP3 | AI truthfulness | A1–A9 |
| SP4 | Automation engine | U1–U6 |
| SP5 | Calendar / telephony / map | C1–C4, F-12, F-13 |
| SP6 | CRM data truth | D1–D3, F-52 |
| SP7 | Backup / invite / settings | B1–B6 |
| SP8 | Remaining security (Medium/Low) | **SP8a** (Auth/RBAC: F-03 [optional], F-06, F-07, F-08, F-09, F-10, F-11, F-14, F-34, F-43, F-45 — F-04/F-05 moved to SP1, see §5)<br>**SP8b** (Network/Transport/CSRF/Uploads: F-16, F-17, F-18, F-19, F-22, F-23, F-24, F-25, F-26, F-27, F-28, F-29, F-30, F-37, F-38, F-39, F-40, F-41, F-42, F-44, F-47)<br>**SP8c** (Integrity/Audit/AI Guard: F-35, F-46, F-48, F-49, F-50, F-51) |
| SP9 | Docs, tests, config truth | P2 list, S1–S5 (exclusive simulation/demo truth), CHANGELOG/README drift |

**Finding-source note:** the round-4 review details only ~33 of its 52 sweep findings
inline. Numbers assigned above but not described in the review or in §12 (F-12, F-13,
F-36, F-52) must have their content pulled from the full sweep transcript **before the
corresponding SP's planning begins** — they are not placeholders to be dropped.

**Definition of done per SP:** `make check` passes; new logic has tests (≥ 50% diff
coverage); migrations follow the `NNNNN_` convention with LF endings and `sqlc generate`
run when queries change; `make check-licenses` green for new dependencies; Conventional
Commits; the `migrations` symlink invariant untouched.

## 4. Cross-cutting principles

1. **No fake success.** Any code path with an external promise returns an honest error
   (HTTP 4xx/5xx + user-facing message) on failure — never `success:true` without work.
   Exception: `DEMO_MODE=true`, which must be visibly labeled as demo in the UI.
   **Demo signal source:** the frontend's `isDemo` flag comes from the minimized
   public `/api/v1/health` response (see SP8 / F-40), which retains exactly
   `{"status":"ok","demo_mode":<bool>}` and drops all AI-config disclosure.
2. **License policy:** new deps must be 100% permissive. Planned: `emersion/go-imap` v2
   (MIT) for IMAP; `emersion/go-message/mail` (MIT) for MIME multipart and attachment
   parsing; SMTP via stdlib `net/smtp` + TLS; Vitest (MIT) for frontend unit tests;
   a CC0/ODbL-attributed PLZ-centroid dataset for geocoding (data license documented
   and shipped alongside).
3. **One secrets mechanism:** new `internal/crypto` package (AES-256-GCM, key file via
   `SECRETS_KEY_PATH` env, auto-generated 0600 if absent). Used by TOTP (SP1) and email
   credentials (SP2). Replaces the caller-less `EncryptPassword` copy in
   `internal/core/email/service.go`.
4. **Auditability:** security-relevant actions (settings export/import, login/logout,
   TOTP events, role changes, backup export/restore, webhook failures) are audit-logged
   with real `ip_address`/`user_agent` — incrementally closing F-49/F-50/F-51.
5. **Memory budget (< 420 MB steady-state target; 2 GB transient ceiling for bounded burst
   operations only):** no heavy in-memory caches; email sync is incremental (UID-based);
   geocoding is an offline lookup table, not a service. Argon2id derivation is bounded by
   semaphore (up to 4 concurrent derivations). Any operation that needs the 2 GB ceiling
   (HNSW index build, backup/restore streaming) must release the memory back down to the
   steady-state target once it completes — verified by an RSS check in the SP's tests.
6. **Explicitly sound — do not regress:** the round-4 review's verified-sound list stays
   untouched: Ed25519 JWT with pinned algorithm; Argon2id with
   `subtle.ConstantTimeCompare`; sqlc-only data access (zero dynamic SQL); the
   DNS-rebinding SSRF guard in `internal/ai/research.go` (reused as the pattern for
   F-28/F-29, not rewritten); no `localStorage` token storage and no
   `dangerouslySetInnerHTML`/`eval` sinks; non-root container; Postgres not port-mapped;
   CSV export formula-injection sanitization.

## 5. SP1 — Security Critical/High

### F-31 / F-32 — settings export/import secret handling
- `ExportedSettings` never emits plaintext secrets. `password`, `api_key`,
  `connector_api_token` are redacted to `has_<secret>: true` flags.
- Rebuild use case stays functional: `ImportSettingsToEnv` already supports
  `keepExistingSecrets=true` — importing a redacted export preserves existing secrets.
- Import response uses the same redaction helper (no secret echo to the client).
- Both endpoints get audit-log entries.

### F-33 — `.env` injection
- Import validation rejects `\n`/`\r` in any submitted value (HTTP 400).
- `GenerateEnvContent` quotes values robustly.

### F-15 — CSRF bypass via `Authorization: Bearer` prefix
- Fail-closed: the auth middleware validates the bearer JWT and sets a context flag
  `auth_via_bearer` **only after successful validation**. `CSRFProtectionMiddleware`
  skips the double-submit check only when that flag is set; cookie-authenticated
  mutations always require the double-submit check.
- **Connector webhook (explicit exemption):** `POST /connectors/lead-intake` uses the
  non-JWT `CONNECTOR_API_TOKEN` and is currently mounted inside the CSRF group,
  passing only via the removed prefix skip (test-enforced in `csrf_test.go`). It is
  re-mounted **outside** the CSRF middleware group with its own constant-time token
  check; rate limiting + audit per F-22 (SP8b) apply. U1 (`NEW_LEAD`) and B6 depend
  on this route staying functional.
- **Legitimate bearer clients today:** none — the frontend authenticates exclusively
  via cookies; the connector webhook uses its own token scheme. The `auth_via_bearer`
  exemption exists for future API clients only.
- Weak predictable fallback token (`hex(time.Now().String())`) removed — token
  generation failure yields HTTP 500 instead. `ClearCSRFCookie` sets `Secure`. This
  is the **single** prescription for `csrf.go`'s fallback/cookie code; SP8 only adds
  session binding on top (see §12).
- **Test requirement:** an explicit test asserts `auth_via_bearer` is never set on a request
  that also carries a valid session cookie plus an attacker-supplied `Authorization` header —
  the flag may only be set when bearer-token validation is the sole successful auth path for
  that request, so cookie-authenticated mutations can never piggyback a bogus header to skip
  the double-submit check.

### F-20 — rate-limit key spoofing
- `chi/middleware.RealIP` removed from `router.go`. IP extraction uses `r.RemoteAddr`
  plus the existing trusted-proxy logic in `internal/auth/limiter.go` for
  `X-Forwarded-For` only. The Caddyfile explicitly strips `True-Client-IP`/`X-Real-IP`
  (defense in depth).

### F-21 — unthrottled login
- `/auth/login` gets an HTTP-level rate limit: 5 attempts/minute per IP + email, with
  exponential backoff on lockout.
- A semaphore caps concurrent Argon2id derivations at 4 (64 MiB each) to prevent
  memory-exhaustion DoS.

### F-01 & F-02 — TOTP plaintext storage & replay (Optional / Opt-In)
- **Policy:** TOTP 2FA is strictly **optional**. Login and authentication work fully via email, password, and rate limiting; no user is forced to configure 2FA.
- **Opt-in Hardening:** If a user chooses to activate 2FA:
  - `internal/crypto` (AES-256-GCM, `SECRETS_KEY_PATH`) encrypts the TOTP secret so `totp_secret_encrypted` is genuinely protected. Lazy migration: on successful TOTP verification with a detected plaintext secret, re-encrypt and persist.
    Detection is explicit, not inferred from length/format: a new `users.totp_secret_encrypted_at TIMESTAMPTZ` column is `NULL` for any pre-existing plaintext secret and set on every encrypt/re-encrypt. The check is `totp_secret_encrypted_at IS NULL`, never a heuristic on the stored value's shape.
  - New migration: `users.totp_last_used_step BIGINT` (RFC 6238 semantics: reject codes from time-steps ≤ last used step) ensures replay protection only for accounts with 2FA enabled.

### F-04 & F-05 — User enumeration & timing attacks (moved from SP8a)
Original review priority (High) outweighs the Medium/Low grouping in SP8a; fixed alongside
the other SP1 auth findings so the login endpoint is enumeration-safe and rate-limited in the
same PR, closing the exposure window instead of leaving it open across SP2–SP7.
- Unknown-user login executes a genuine constant-time Argon2id calculation against a fixed
  dummy hash (same parameters as real accounts) before returning — no early-return short
  circuit on "user not found".
- Deactivated/invited/locked accounts return the same uniform `401 Unauthorized` body and
  timing profile as a wrong-password result on an active account.

## 6. SP2 — Real email (SMTP/IMAP)

**Architecture:** new package `internal/mailclient` with two probes:
- `SMTPSender` — stdlib `net/smtp`, STARTTLS/SMTPS, per-account credentials.
- `IMAPFetcher` — `emersion/go-imap` v2, incremental UID-based sync.

`internal/core/email` becomes the orchestrator in front of both.

- **Account model:** existing account table extended with type (team/personal),
  owner user ID, sync state, last-seen UID per folder. Team mailboxes are
  admin-managed; personal mailboxes belong to a single user. Credentials encrypted
  via `internal/crypto`. CRUD: `POST/GET/PATCH/DELETE /api/v1/emails/accounts` plus
  `POST /emails/accounts/{id}/test` performing live IMAP-login + SMTP-AUTH checks.
- **Send (E1):** real `POST /api/v1/emails/send` (account, to, cc, subject, body);
  message persisted as `direction=outbound` with thread mapping. `InboxPage`'s
  `handleSendReply` calls it — end of the network-less "versendet!".
- **Sync (E6/E7):** `EmailSyncWorker` does a real incremental IMAP fetch per active
  account, maps senders to contacts, groups threads via `Message-ID`/`In-Reply-To`.
  River periodic job every 60 s (matches the UI's "Auto-Sync alle 60 Sekunden") plus a
  manual trigger endpoint. `EmailTriageWorker` actually calls the AI gateway (with PII
  guard) and is enqueued after sync ingests new messages.
- **AI draft (E2):** the inbox reply draft calls the real `POST /api/v1/ai/triage`
  (`draft_reply`) instead of `setTimeout` with fixed text.
- **Tags (E3):** `tags JSONB` on `email_messages` + `PATCH /emails/{id}`; persistent
  instead of component state.
- **Demo inbox (E5):** the auto-created default account exists only in `DEMO_MODE`.
- **Attachments:** the orphaned `email_attachments` table is written via
  `StorageService` (SHA-256 dedup — making the README claim true), uploads bounded by
  `http.MaxBytesReader`.
- **UI (E4):** the "E-Mail-Konten" settings tab becomes real CRUD with per-account
  test-connection status.
- **Tests:** interface-boundary fakes — in-test SMTP/IMAP mini-servers over
  `net.Listener`; no real mail server in CI.

## 7. SP3 — AI truthfulness

- **A1:** delete `simulateGemmaResponse`. Gateway failure → typed 502 with an honest
  message ("KI-Dienst nicht erreichbar"); the leaked `resp.Body` on the non-200 path is
  closed. No `err == nil` path with invented text.
- **A2:** delete `generateSmartFallback` and `isGenericSimulatedReply`; chat responds
  honestly "nicht verfügbar" instead of 11 keyword-matched paragraphs with fabricated
  legal text.
- **A3:** remove the fabricated domain/industry synthesis; wire `researchSvc` into
  `ChatService` — real data or an honest "keine Daten verfügbar".
- **A4:** `POST /ai/synthesize-notes` reads the request body, loads the actual notes,
  prompts the gateway, returns the real analysis. The UI creates deals only from
  model-returned values with explicit user confirmation — no hardcoded "85 % Kaufintent"
  / €22,500 / due date; AI unavailable → honest error.
- **A5:** `ParseBill` drops the "Familie Müller" template; `document_text` is required
  (400 if empty); `OfferCalculatorModal` gets a real file-upload/text-paste control.
- **A6:** `ListResearchJobs` returns the persisted `result_json` (empty until done);
  `ResearchCompany` persists its result.
- **A7:** `Gateway.Generate` uses `SanitizeInputWithCount`; `PIIFilterTriggered` counts
  real filter events in chat, triage, and research.
- **A8:** observability reads provider/model/compliance standard from actual config.
- **A9:** real embeddings: Ollama embeddings API (`OLLAMA_EMBEDDING_MODEL`, default
  `nomic-embed-text`, 768 dims); migration adds a `vector(768)` column + HNSW index to
  knowledge chunks with an `embedding_model` tag for re-embedding on model change; semantic
  search via pgvector cosine distance; Settings UI shows the real model and real chunk
  count.
  **Dimension-change handling:** `vector(768)` is fixed at migration time and cannot hold a
  different-dimension model's output. Switching `OLLAMA_EMBEDDING_MODEL` to a model whose
  output dimension differs from the column is rejected at startup with an explicit config
  error naming the mismatch — it is a new migration (`NNNNN_resize_embedding_vector.sql`,
  `ALTER COLUMN embedding TYPE vector(<n>)`) plus a full re-embed, not a same-schema
  re-embed job. The re-embed River job handles same-dimension model swaps only.

## 8. SP4 — Automation engine

- **U1 — wire `TriggerEvent`:** `NEW_LEAD` after connector lead-intake create;
  `DEAL_WON` in the deal update handler on stage change; `INBOUND_EMAIL` after sync
  ingest; `INACTIVITY_TIMEOUT` as a River periodic job scanning inactive contacts.
- **U2:** `SET_TAG` writes a real tag: `tags JSONB` on `contacts`/`deals`; the action
  appends the tag.
- **U3:** `DRAFT_EMAIL` generates a real draft via the AI gateway (PII-guarded) and
  stores an `email_messages` row with `is_draft`.
- **U4:** `NOTIFY_USER` resolves the entity's actual owner/assignee — never a contact
  UUID as `user_id`.
- **U5:** default workflows become real seeded DB rows (migration,
  `ON CONFLICT DO NOTHING`), no hardcoded back-dated fixtures.
- **U6:** UI "Testlauf" calls a new `POST /automations/{id}/test-run`; "Schritt
  freigeben" calls the existing `POST /automations/runs/{id}/approve`. Toasts show
  real results/errors.

## 9. SP5 — Calendar / telephony / map

- **C1 — remove fake Graph/Exchange/Google push:** Remove all fabricated external sync
  promises and UI badges ("M365 & Google OAuth verbunden", provider badges) from
  `CalendarPage.tsx` and e2e test specs. Deprecate/remove the mock `PushExternal` endpoint
  (`POST /appointments/{id}/push-external` returning fabricated `provider: microsoft_and_google`).
  Calendar is maintained as a focused, robust local CRM calendar without external cloud OAuth overhead.
- **C2 — real RFC 5545 ICS download & invites:** `GET /api/v1/appointments/{id}/ics` returns 404
  for unknown IDs, enforces ownership, and generates standard-compliant RFC 5545 `.ics` data
  with valid timestamps, summary, location, and UID. Fully compatible with Outlook, Apple Calendar,
  Google Calendar, and Thunderbird via calendar import/attachment.
- **C3:** telephony is honest — DB insert failure → 500 (no fabricated 201 with an
  in-memory record); the call modal is labeled "Anruf-Protokoll" plus a real `tel:`
  link for click-to-call.
- **C4:** offline PLZ-centroid geocoding (bundled dataset, license documented),
  server-side on contact create/update; optional external provider via config
  (opt-in, PII noted in docs). `MapView` plots real coordinates; contacts without a
  resolvable location get an honest "kein Standort" indicator — no random scatter.

## 10. SP6 — CRM data truth

- **D1:** consent and energy fields (`consent_phone`, `consent_email`,
  `consent_updated_at`, `zaehlernummer`, `stromverbrauch_kwh`, `gasverbrauch_kwh`,
  `eigentuemer_status`) in create/update request structs and sqlc queries;
  `consent_updated_at` set on change with an audit entry (UWG-relevant); CSV export
  writes the real values.
- **D2:** §355 BGB revocation: a "Widerruf eintragen" deal action sets
  `widerrufen_at`/`widerruf_grund`; Kanban gains a "Widerrufen" stage; the report
  metric computes from real data.
- **D3:** migration adds `old_values`/`new_values JSONB` to `audit_logs`; the audit
  service populates them wherever the prior row is loaded.

## 11. SP7 — Backup / invite / settings

- **B1:** export covers **all** tables as a true stream (chunked writes, no RAM
  accumulation) and a **restore endpoint** is added (upload JSON, transactional,
  dependency-ordered inserts, admin-only, audit-logged, merge/replace mode).
- **B2:** restore drill becomes real: backup into a scratch schema, row-count + FK
  verification, honest result.
- **B3:** invite flow with a one-time token: the admin receives a **copyable invite
  link** in the UI (works in every deployment, no mailbox required); optionally sent
  by email when an SP2 mailbox is configured. `GET /auth/accept-invite?token=…` lets
  the user set their own password and become `ACTIVE`.
- **B4:** the 2FA toggle calls the real `POST /auth/totp/disable` with password
  confirmation.
- **B5:** email templates become DB-backed CRUD; the template generator calls the real
  AI chat endpoint.
- **B6:** the test-webhook sends the correct `Authorization: Bearer` header and a
  schema-conformant `LeadIntakePayload`; the result is displayed honestly.

## 12. SP8 — Remaining security findings (Medium/Low)

SP8 decomposes into three focused, independently reviewable and testable sub-PRs:

### SP8a — Authentication, Sessions & RBAC
- **F-03 (TOTP setup race):** Write secret to DB only upon successful initial verification, preventing unverified secrets in storage.
- **F-04 & F-05:** moved to SP1 (§5) — fixed with the other Critical/High auth findings instead of here.
- **F-06 & F-07 (Session revocation & refresh reuse detection):** Revocation is persisted in PostgreSQL (`revoked_tokens` table) and checked in `AuthMiddleware`. Single-use refresh token rotation with family revocation on reuse detection. User role/status changes immediately revoke all active tokens of the target user.
- **F-08 (Password policy):** Validate min. 8 chars on user registration and admin-initiated user creation, not just on password change.
- **F-09 (Fail-closed ownership):** Fix pattern `if claims != nil && ...` to fail closed (`if claims == nil || ...`).
- **F-10 & F-11 (Data isolation & Notes ownership):** Ownership enforcement on contacts, companies, and notes. Note ownership verified via immutable `user_id` instead of loose email string.
- **F-14 (Ungated routes):** Role guards applied to previously open routes (`/emails/*`, `/telephony/calls`, `/automations`, `/notifications/*`).
- **F-34 (Bootstrap credentials):** Bootstrap admin password is not printed to stdout/container logs; `users.must_change_password` flag forces immediate password reset upon first login.
- **F-43 (Response body token leakage):** Remove raw JWT access/refresh tokens from JSON response bodies of `/auth/login` and `/auth/refresh`; authentication relies exclusively on secure HttpOnly cookies.
- **F-45 (Auth guard alignment):** Frontend route guards align strictly with authenticated session state.

#### Defined RBAC Role Matrix
| Route Group / Domain | ADMIN | VERTRIEB | BACKOFFICE | BENUTZER |
|---|---|---|---|---|
| Contacts / Companies | Full CRUD | Full CRUD | Full CRUD | Read-only + own assigned |
| Deals / Kanban | Full CRUD | Full CRUD | Edit/Update (no delete) | Read-only (assigned) |
| Notes / Todos / Appointments | Full CRUD | Full on assigned/team | Full on assigned/team | Own items only |
| Emails (Inbox, Send, Sync) | Full (team + personal) | Team + personal | Team + personal | Personal mailbox only |
| Telephony & AI Copilot | Full | Full | Full | None |
| Reports & CSV Export | Full | View reports | View reports + CSV export | None |
| Settings / Backup / Connectors / Users / Audit | Full | None | None | None |
| Automations (workflow config, runs, test-run, approve) | Full | Runs: view + approve, test-run | Runs: view | None |
| Notifications | Own | Own | Own | Own |

---

### SP8b — Network, Transport, CSRF, Uploads & Server Hardening
- **F-16, F-18, F-19, F-44 (CSRF hardening):** Double-submit CSRF token bound to session
  via HMAC. The SP1 end-state is the single baseline (§5/F-15): **no fallback at all** —
  token-generation failure yields HTTP 500 — and `Secure` on all cookie paths. SP8b adds
  only the session binding and the frontend typed cookie parser (replacing the loose regex).
- **Honest failure paths (review's unnumbered Medium findings):** `core/reports` and
  `core/export` return errors instead of empty success; unbounded `Limit: 10000` reads
  surface truncation warnings. If these carry F-numbers in the full sweep transcript,
  fold them in during SP8 planning.
- **F-17 (Login CSRF):** Explicitly document login CSRF exemption, backed by strict IP + email rate limiting.
- **F-22 & F-23 (Rate limiting for Webhooks & Heavy Endpoints):** Rate limiting and audit logging on `/connectors/lead-intake` (demo token only allowed in `DEMO_MODE`); rate limit export/backup endpoints.
- **F-24 & F-25 (Upload MIME validation):** Strict MIME allowlist (`application/pdf`, `image/png`, `image/jpeg`, `text/plain`, `text/csv`) replacing blocklist; validation executed prior to content-addressable storage (CAS) dedup cache checks.
- **F-26 (CSV Import integrity):** Validate headers, reject malformed rows with line numbers, eliminate positional trust.
- **F-27 (Information leakage in errors):** Centralized error responder preventing internal Postgres/Go `err.Error()` details from leaking to clients.
- **F-28 & F-29 (SSRF guard):** Expand SSRF protection to cover IPv4-mapped IPv6 (`::ffff:127.0.0.1`), multicast, and reserved CIDRs for `OLLAMA_BASE_URL` and `AI_BASE_URL`.
- **F-30 (Self-updater integrity):** Download only tagged GitHub release assets accompanied by verified `SHA256SUMS` checksums before replacing the binary.
- **F-37, F-38, F-39 (Transport & Cookies):** Security headers (CSP, HSTS, Permissions-Policy) enforced at the Go application layer; `Secure` cookie flag fail-closed; `SameSite=Strict` on access token cookies.
- **F-40 (Health endpoint):** `/api/v1/health` remains open and accessible for local single-tenant hosting; provides system status and `demo_mode: <bool>` required by the unauthenticated frontend login screen.
- **F-41 (SSE auth + per-user routing):** Move `/events/stream` into the protected API
  router using standard `AuthMiddleware`, **and** add per-user routing to the SSE hub:
  clients subscribe on a per-user channel keyed by the authenticated user ID, and the
  notification service delivers each notification only to its target user's channel —
  ending the cross-user broadcast of `title`/`message` to all connected sessions
  (`internal/sse/hub.go`, `internal/core/notification/service.go`).
- **F-42 (Local binding):** Docker compose for local development binds `crm-server`, `crm-worker`,
  and `crm-db` ports to `127.0.0.1` rather than public `0.0.0.0`. `crm-proxy` (Caddy) keeps its
  public `80`/`443` bind — it is the only intended public entry point; the fix removes accidental
  direct exposure of the backend/db ports, not the proxy itself.
- **F-47 (HTML Sanitization):** Enforce safe contextual HTML escaping for user-supplied strings.
- **Server Timeouts & Size limits:** Set standard 30s `WriteTimeout` on server; override write deadline specifically for SSE via `http.ResponseController`; apply `http.MaxBytesReader` (5 MB default, tailored for uploads) on request bodies.

---

### SP8c — Integrity, Audit Logging & AI Guarding
- **F-35 (AI Observability):** Record acting `user_id` in `AIAuditLog` alongside model metadata.
- **F-46 (Prompt injection defense):** Sanitize chat history role markers; isolate system prompt from user transcripts.
- **F-48 (PII Guard refinement):** Refine PII redaction regexes to prevent false-positive over-broad redaction.
- **F-49, F-50, F-51 (Cryptographic Audit Log):** Implement hash-chaining (`prev_hash`, `hash`) and append-only semantics for `audit_logs`; ensure security-relevant events (login, logout, TOTP, role change, settings export, backup) are audit-logged with real `ip_address` and `user_agent`.

## 13. SP9 — Docs, tests, config truth

- **Dead endpoints:** wire the valuable ones (`/auth/refresh` via a frontend
  interceptor — sessions survive past 30 minutes; `/auth/totp/disable`; automations
  approve; ICS download; email threads; solar-calculation; `GET /{id}` detail routes
  where the UI needs them); remove the rest.
- **Orphaned tables:** `email_attachments` used by SP2; `company_research`,
  `intake_forms`, `workflow_steps` either wired or dropped via migration.
- **⌘K search** extended to appointments and emails; `StorageService` real usage via
  SP2 attachments.
- **Tests:** Vitest for new frontend logic; an E2E docker-compose profile running
  against real Postgres; the fake-asserting E2E specs (calendar/telephony, automations,
  inbox) rewritten to assert real persistence round-trips.
- **Simulation/demo (S1–S5):** `SimulationWidget` lazy-loaded and mounted only in
  `DEMO_MODE`; `isDemo` defaults to `false` (no `demo123` prefill without demo mode);
  the login page hides demo credentials in production; reports drop fixed strings.
- **Config drift:** a single default model name from one config package — the default
  becomes the real Ollama tag `gemma3:12b` (replacing the nonexistent `gemma4:12b`);
  launcher env names aligned with `docker-compose.yml`/`.env.example`.
- **Docs truth pass:** README/CHANGELOG vaporware lines corrected;
  `docs/AUDIT_REPORT_2026-09-21.html` removed from the repo and its demo credentials
  rotated.

## 14. Testing strategy

- Every SP adds Go unit tests for new logic; external protocols tested against
  interface-boundary fakes (in-test SMTP/IMAP/HTTP mini-servers).
- Coverage gate ≥ 50% diff per PR (pre-push enforced).
- `make check-licenses` on every dependency change.
- E2E extended per SP; real-DB E2E profile lands in SP9.
- A final review pass (round 5) re-runs the fake-success sweep to verify zero findings.

## 15. Risks & mitigations

| Risk | Mitigation |
|---|---|
| `SECRETS_KEY_PATH` lost → TOTP/mail secrets unrecoverable | Document backup of the key file in ops docs; clear startup error if key missing and secrets exist |
| IMAP server diversity (folder naming, flags) | Sync confined to INBOX initially + configurable folder; UIDVALIDITY check to re-sync safely |
| Embedding model change invalidates vectors | `embedding_model` tag per chunk; re-embed River job on same-dimension model change; dimension-changing model swap rejected at startup, requires an explicit `ALTER COLUMN` migration (see SP3/A9) |
| 2 GB transient ceiling silently becomes the new resting baseline | RSS check in tests for burst operations (HNSW build, backup streaming) confirms memory returns to the < 420 MB steady-state target after completion |
| Big-bang regressions across 9 SPs | One PR per SP, `make check` + coverage gate each; sequential order lets later SPs build on earlier ones |
| New deps violate license policy | `make check-licenses` mandatory per SP; pre-commit license audit on go.mod/go.sum changes |
| Restore endpoint becomes an attack surface | Admin-only + audit + transactional + size limits (`MaxBytesReader`) |

## 16. Migration & dependency summary

**Migrations (new, sequential, LF):** `users.totp_last_used_step`; `users.must_change_password`;
`email_accounts` type/owner/sync columns; `email_messages.tags`; `contacts.tags` + `deals.tags`;
`audit_logs.old_values/new_values` + `audit_logs` hash chain (`prev_hash`, `hash`);
knowledge-chunks `embedding vector(768)` + HNSW index + `embedding_model`; `users.totp_secret_encrypted_at`;
email-template table; invite-token columns/table; workflow seed inserts; `revoked_tokens` table (persisted
session revocation + single-use refresh token reuse detection, F-06/F-07). A later, conditional
`NNNNN_resize_embedding_vector.sql` is added only if/when the embedding model's output dimension
changes (see SP3/A9) — not part of the initial batch.

**New dependencies (all permissive, verified via `make check-licenses`):**
`github.com/emersion/go-imap/v2` (MIT), `github.com/emersion/go-message/mail` (MIT), Vitest (MIT,
dev-only). SMTP via stdlib. PLZ-centroid dataset shipped as data with documented
license/attribution.
