# Security & Feature Review — Round 4 (2026-09-21)

> **Scope:** Follow-up review of `openlocalcrm` at HEAD `f890797` (v1.0.6), after round-1
> (`docs/REPO_REVIEW_2026-09-21.md`), round-2 (`docs/REPO_REVIEW_2026-09-21_round2.md`) and
> round-3 (`docs/SECURITY_REVIEW_2026-09-21_round3.md`) remediation. Focus per request:
> **feature correctness / anti-fake sweep** (does a feature actually do what it claims, or
> only simulate success) **and security**, across the whole repository.
> **Method:** 3 parallel independent full-repo sweeps — (1) exhaustive fake/simulated/stubbed
> feature hunt across backend and frontend, (2) security reconnaissance of authN/authZ, CSRF,
> rate limiting, injection/upload, SSRF, secrets, transport, AI guard, and audit logging,
> (3) feature-completeness cross-reference of README claims vs. routes vs. frontend calls vs.
> data model vs. test coverage — plus manual file:line verification of every claim before
> inclusion here.
> **Status:** Read-only audit. No code changed. For processing: see
> `docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md`.

---

## Executive summary

Round 3's remediation (`f890797`, v1.0.6) fixed a meaningful slice of what it claimed — the
deal IDOR (N1/N2), several silent-failure bugs (N3/N4/N7), and the CSV-import fabrication
(N12) are genuinely resolved. But the review that produced round 3 undercounted the problem's
scope. This round finds:

1. **The core defect is systemic, not confined to AI/automation.** Nearly every subsystem that
   depends on an external protocol or service — e-mail (SMTP/IMAP), telephony, calendar
   push, geocoding, KI embeddings/RAG, calendar invites — reports success to the user while
   performing **no real external work**. There is **zero** SMTP/IMAP code anywhere in the
   repository despite an entire "E-Mail-Konten (IMAP/SMTP)" settings tab, a "reply sent"
   confirmation, and an AI email-drafting feature.
2. **Round 3's own "fixed" list contains at least three items that are not fixed.** CHANGELOG
   1.0.6 claims removal of the Microsoft/Google calendar-push promises (still present verbatim
   in `CalendarPage.tsx`), a corrected pgvector claim (still shown in Settings and still
   entirely unimplemented), and a working `ApproveStep`/HITL flow (the approve button in the
   UI never calls it).
3. **A new Critical-severity finding**: `GET /api/v1/settings/export` returns the plaintext
   initial admin password (in launcher/native deployments) plus the AI provider API key and
   the connector webhook token, to any ADMIN-authenticated request — and that authentication
   can itself be forced cross-origin because CSRF protection is bypassed by the mere presence
   of an `Authorization: Bearer` header prefix, without validating the token (F-15 → F-31
   chain).
4. **Two new High-severity findings undermine the whole rate-limiting model**: the login
   endpoint has no HTTP-level rate limiter at all, and the IP used as the rate-limit key is
   taken from `chi/middleware.RealIP`, which trusts client-supplied `True-Client-IP` /
   `X-Real-IP` headers that the deployed Caddy configuration does not strip — making every
   throttle in the codebase trivially bypassable by header rotation.
5. **The AI/automation fake-success pattern that rounds 2–3 partially addressed is still the
   largest single cluster of findings**: a silent canned-response fallback in the LLM gateway,
   a second layer of hardcoded keyword-matched replies in chat, a 100%-fabricated
   note-synthesis endpoint that can create real €22,500 deals from invented "85% buying
   intent", dead-code automation triggers, and a knowledge base that claims pgvector semantic
   search while performing no embedding at all.
6. **The core CRM data-truth promises are unmet**: consent checkboxes (UWG §7) and energy-data
   fields exist as DB columns and are collected by the UI, but have no write path anywhere in
   the request→service→SQL chain, so they are silently dropped; the §355 BGB revocation stage
   and its reporting metric are structurally unreachable for the same reason.

---

## P0 — Fake / simulated / fabricated features

Legend: **Claim** = what the UI/API presents · **Reality** = what the code does.

### E-mail — no SMTP/IMAP implementation exists anywhere in the repository

`grep -rn "net/smtp\|smtp\.\|go-imap\|emersion\|gomail\|SendMail" internal/ cmd/` → **0 hits**;
`go.mod` lists no mail library.

| # | file:line | Claim | Reality |
|---|---|---|---|
| E1 | `web/src/pages/InboxPage.tsx:249-254` | "Antwort erfolgreich an … versendet!" | `handleSendReply` makes zero network calls. **No send endpoint exists anywhere in `router.go`.** |
| E2 | `InboxPage.tsx:220-247` | "KI-Antwortentwurf mit Gemma 12B" | `setTimeout(600)` + hardcoded German text incl. a fixed "30 kWp + 20 kWh" regardless of the actual email. The real `POST /api/v1/ai/triage` (`router.go:384`) returns `draft_reply` and is never called from here. |
| E3 | `InboxPage.tsx:119,188-204` | Dynamic e-mail tagging/flagging | Tags live only in `useState<Record<string,string[]>>({})`; lost on reload; no table, no endpoint. |
| E4 | `web/src/pages/SettingsPage.tsx:794-840` | "E-Mail-Konten (IMAP/SMTP Synchronisation)" — "1 Konto aktiv", "IMAP: SSL/993", "SMTP Host: mail.openlocalcrm.local:587", "Auto-Sync Intervall: Alle 60 Sekunden" | Entirely static JSX. No query, no CRUD, no auto-sync loop. |
| E5 | `internal/core/email/service.go:50-61` | (implicit) a functioning inbox account | Auto-creates a fake "Default Demo Inbox" account on `localhost` with an empty password and `IsActive: true` — **in production, not just demo mode**. Called from `IngestMessage` (`:83-88`). |
| E6 | `internal/queue/jobs.go:23-39` | `EmailSyncWorker` — "Go River Queue Daemon … E-Mail IMAP Sync" (README:49) | No-op: `log.Printf("[worker] syncing … "); return nil`. Comment at `:34`: "real IMAP sync would connect and fetch here." |
| E7 | `internal/queue/jobs.go:53-68` | `EmailTriageWorker` — AI classification worker | No-op: comment at `:66` "Real triage would call the AI gateway here; in demo mode this logs the action." Also: **nothing in the codebase ever enqueues either job** (`grep "EmailSyncArgs{|EmailTriageArgs{"` → 0 hits outside definitions). |
| E8 | `internal/core/email/service.go:171-235` | (implicit) encrypted stored SMTP/IMAP credentials | `EncryptPassword`/`DecryptPassword` (real AES-256-GCM) exist but have **zero callers**; no `EMAIL_ENCRYPTION_KEY` env var exists anywhere. |

### AI — silent canned-response fallback masquerading as model output

| # | file:line | Claim | Reality |
|---|---|---|---|
| A1 | `internal/ai/gateway.go:145-149,187-191` → `simulateGemmaResponse` (`:204-249`) | An answer from the configured Gemma/OpenAI model | On any error or non-200 response, returns hardcoded German text **with `err == nil`**. Callers cannot distinguish real from fabricated. Round-3 finding M5, listed "fixed" in CHANGELOG 1.0.6 — **verified still present, unfixed.** Also leaks the response body (`resp.Body` never closed on the non-200 early return). |
| A2 | `internal/ai/chat.go:162-184,214-319` → `generateSmartFallback` | Copilot chat answer | Second fake layer: 11 `strings.Contains` keyword branches returning fixed German paragraphs, including hardcoded §355 BGB (`:300`) and §7 UWG (`:305`) legal text. `isGenericSimulatedReply` (`:208-212`) detects its own fake output by matching the literal string `"106.700 €"` left over from an earlier fake dataset. |
| A3 | `internal/ai/chat.go:452-489` | "Recherchierte Unternehmensdaten & Potenzial" | Fabricated: domain = `name + ".de"`, industry is always "Handwerk / Gewerbe & Lebensmittel" regardless of the actual company. `researchSvc` is never wired into `ChatService`. |
| A4 | `internal/server/handlers/notes.go:158-185` — `POST /api/v1/ai/synthesize-notes` | AI-generated note synthesis with buying-intent scoring | **100% fabricated.** Never reads the request body, never touches the notes, never calls the AI gateway. Returns "SEHR HOCH (85%)" buying intent for every customer and suggests a fixed €22,500 deal — which the UI (`web/src/components/notes/ActivityLogDrawer.tsx:146-176`) then **actually creates** in the CRM. Hardcoded due-date `2026-08-28`. |
| A5 | `internal/server/handlers/ai.go:191-212` — `ParseBill` | "OCR-Dokumentenextraktion mit Gemma 12B" | Default branch returns a fixed "Familie Müller" template incl. a fabricated meter number. The only UI caller (`web/src/components/calculator/OfferCalculatorModal.tsx:129-135`) **has no file-upload control** and never sends `document_text`, so the AI-extraction branch is structurally unreachable. Round-3 finding N6 — **verified still unfixed.** |
| A6 | `internal/server/handlers/ai.go:336-340` — `ListResearchJobs` | Persisted company-research results | Fabricates the result payload for **every** stored job on every read; the real research output (from `ResearchCompany`, when it does run) is never persisted to `result_json`. |
| A7 | `internal/ai/chat.go:202`, `internal/ai/triage.go:126`, `internal/ai/research.go:169` | "PII-Schutz aktiv" (EU AI Act observability) | `PIIFilterTriggered: false` hardcoded in all three. `Gateway.Generate` calls the count-discarding `SanitizeInput` (`gateway.go:86-87`) instead of `SanitizeInputWithCount`, so the "PII blocked" compliance metric is structurally always 0. |
| A8 | `internal/ai/observability.go:160-162` | Live compliance dashboard reflecting actual configuration | `ComplianceStandard`, `ActiveModel: "gemma4:12b"`, `Provider: "Ollama (Local On-Premise)"` are hardcoded literals — shown even when the deployment is configured for OpenAI/`gpt-4o-mini`. |
| A9 | `internal/server/handlers/ai.go:230` + `SettingsPage.tsx:1541` | "In pgvector vektorisiert (384-dim Embeddings)" / "PostgreSQL 16 + pgvector" | `ChunksCount: 4` is a fixed literal. **No embedding generation, no vector column, no vector search exists anywhere.** `CREATE EXTENSION vector` (`00001_initial_schema.sql:5`) is never used by any table. Round-3 finding N13 — CHANGELOG 1.0.6 claims this was replaced with "honest document indexing"; the UI string in Settings still makes the pgvector claim. |

### Automations engine

| # | file:line | Claim | Reality |
|---|---|---|---|
| U1 | `internal/core/automation/service.go:215-280` — `TriggerEvent` | Workflows fire automatically on `NEW_LEAD`, `DEAL_WON`, `INBOUND_EMAIL`, `INACTIVITY_TIMEOUT` (UI shows these as "AKTIV") | **Dead code.** Grep for callers returns only the function's own definition — nothing in the codebase ever invokes it. |
| U2 | `service.go:374-394` — `SET_TAG` action | Applies a tag to the target contact/deal | Writes an audit-log row only. The tag is never attached to any record. |
| U3 | `service.go:431-451` — `DRAFT_EMAIL` action | Drafts an email for the target | Writes an audit-log row only. No draft text is generated, no message row created. |
| U4 | `service.go` (`NOTIFY_USER` action, near `:455-470`) | Notifies the responsible user | Sets `notifications.user_id` to the **target entity's UUID** (a contact or deal ID), not to any actual user — the notification is unreachable by any account. |
| U5 | `service.go:92-168` | Three "active" default workflows shown when the workflow list is empty | Hardcoded fixtures with back-dated `CreatedAt` timestamps, not real seeded/persisted workflows. |
| U6 | `web/src/pages/AutomationsPage.tsx:86-91` ("Testlauf") and `:278-290` ("Schritt freigeben") | Manually trigger a workflow / approve a HITL step | Both are toast-only — **zero API calls**, even though a working `POST /api/v1/automations/runs/{id}/approve` (`router.go:443`) exists and is simply never called from the UI. |

### Calendar / telephony / map

| # | file:line | Claim | Reality |
|---|---|---|---|
| C1 | `internal/core/appointment/service.go:353-374` — `PushExternal` | "erfolgreich in Ihren Microsoft 365 & Google Kalender übertragen" (`handlers/appointments.go:154-166` returns `{"status":"synced","provider":"microsoft_and_google"}`) | Sets a boolean `is_pushed = true`. **No Microsoft Graph or Google Calendar client exists anywhere in the repository.** CHANGELOG 1.0.6 claims this UI promise was removed — it is still present verbatim in `CalendarPage.tsx:207,210,245,328-330`, and `e2e/calendar-and-telephony.spec.ts:8,15,32` still asserts on it. |
| C2 | `handlers/appointments.go:170-181` — `DownloadICS` | An ICS file for the requested appointment | On a not-found ID, fabricates a generic "Kundentermin" tomorrow and returns HTTP 200 instead of 404. No ownership check on this route at all. |
| C3 | `web/src/components/telephony/CallModal.tsx` | "Click-to-Call Telefonie" | Dials nothing — a stopwatch plus a notes form that POSTs a log row. `internal/core/telephony/service.go:73-83`: **if the DB insert fails, a fake in-memory record with id `call-<nanos>` is fabricated and HTTP 201 success is still returned.** |
| C4 | `web/src/pages/MapViewPage.tsx:52-53` | "D2D & Außendienst-Gebietskarte" — real customer/lead locations | Contacts without stored coordinates are plotted at `50.1109 + (Math.random()-0.5)*0.08` / analogous longitude — **randomized around Frankfurt on every render.** No geocoding exists anywhere; no UI path ever sets `latitude`/`longitude`. |

### CRM data truth (legally relevant)

| # | file:line | Claim | Reality |
|---|---|---|---|
| D1 | `internal/server/handlers/contacts.go:23-35` + `internal/db/queries/contacts.sql:3-20,46-60` | "Getrennte, auditierbare Einwilligungshäkchen für Telefon und E-Mail (UWG §7)" (README:77) | `consent_phone`, `consent_email`, `consent_updated_at`, `zaehlernummer`, `stromverbrauch_kwh`, `gasverbrauch_kwh`, `eigentuemer_status` exist as columns (`00004_energy_d2d_and_consent.sql:6-12`) but have **no write path** in `CreateContactRequest`/`CreateContact` SQL. `ContactsPage.tsx:98,614` sends `consent_phone` — it is silently dropped. CSV export writes constant `false` (`core/export/service.go:80-81`). |
| D2 | `deals.widerrufen_at`/`widerruf_grund` (`00004_energy_d2d_and_consent.sql:16-17`) | "§355 BGB Widerrufsquote" statistic (README:111) | Only ever `SELECT`ed, never `UPDATE`d anywhere. The Kanban has no "Widerrufen" stage (`DealsPage.tsx:35-59`), so the metric in `core/reports/service.go:69-70` is structurally always 0. |
| D3 | `audit_logs` table | "Revisionssicheres Audit-Log … mit JSONB-Diffs (`old_values`, `new_values`)" (README:82) | Single `changes JSONB` column (`00002_core_crm_schema.sql:87-97`); no `old_values`/`new_values` columns exist. |

### Backup, invites, settings

| # | file:line | Claim | Reality |
|---|---|---|---|
| B1 | `internal/server/handlers/backup.go:74-171` — `Export` | "Vollständiger JSON-Export" (README:117), "Streaming-Export in 1.000er-Batches ohne Speicherüberlauf" (CHANGELOG 1.0.5) | Covers **4 of ~19 tables** (companies, contacts, deals, todos only — missing users, notes, appointments, emails, call_activities, notifications, workflows, workflow_runs, audit_logs, knowledge_base_articles, ai_research_jobs). Everything is accumulated in RAM before writing — not a stream. **There is no restore/import endpoint at all.** |
| B2 | `handlers/backup.go:34-73` — "Restore-Drill" | "Automatisierte Datenbank-Integritätsprüfung" | Runs 4 `SELECT`s; if none error, hardcodes `"integrity_check":"passed"` and the sentence "Alle Tabellen und Referenzen sind lesbar und konsistent." Nothing is backed up, restored, or referentially verified. |
| B3 | `internal/server/handlers/users.go:79-128` — `Invite` | "Benutzer einladen" | Generates a random password, **discards it**, sets status `INVITED`. `internal/auth/service.go:143-145` refuses login for any status but `ACTIVE`. **Invited users can never log in** — there is no invite e-mail (impossible per E1-E8) and no accept-invite flow. |
| B4 | `SettingsPage.tsx:995` | 2FA disable toggle | `onClick={() => setTotpEnabled(!totpEnabled)}` — pure local state. `POST /api/v1/auth/totp/disable` (`router.go:284`) is never called. |
| B5 | `SettingsPage.tsx:457-529` | "E-Mail-Vorlagen" + "KI-Template-Generator per Chat (§5.1)" | Hardcoded state arrays; the generator is `setTimeout(1000)` returning one fixed template regardless of input prompt. |
| B6 | `SettingsPage.tsx:531,540-571` | "Test-Webhook" self-test | Broken by construction: sends header `X-Connector-Token`, but the server requires `Authorization: Bearer <token>` (`handlers/connectors.go:20-25`) → always 401; payload schema also mismatches `LeadIntakePayload`. |

### Miscellaneous simulation

| # | file:line | Claim | Reality |
|---|---|---|---|
| S1 | `web/src/simulation/SimulationWidget.tsx`, mounted unconditionally at `web/src/components/layout/AppLayout.tsx:7,44` | Demo-only simulation | Statically imported and unconditionally mounted → **compiled into every production bundle** (`web/embed.go:11` `//go:embed dist/*`). Only hidden at runtime by `if (!isDemo) return null` (`:297`), gated on the unauthenticated `/api/v1/health` response. |
| S2 | `SimulationWidget.tsx:259-268` | "Deal … erfolgreich auf GEWONNEN verschoben" | Makes **zero** API calls — pure fabricated success log naming a deal that may not exist. |
| S3 | `SimulationWidget.tsx:282-284` | Simulation error handling | Any thrown error is converted into a success-styled ticker line with the fallback word "Ausgeführt" (= "executed"). |
| S4 | `web/src/pages/LoginPage.tsx:9,12` | Production login form | Password field pre-filled with `demo123`; `isDemo` state **defaults to `true`** — if `/health` fails to respond, production shows demo credentials. |
| S5 | `web/src/pages/ReportsPage.tsx:66,103` | "Echtzeit-Berechnung" | Fixed "+4.2% über Zielkorridor" text shown for any non-zero conversion rate; hardcoded 100k€ chart-scale ceiling. |

---

## P1 — Security findings (F-01…F-52)

### Critical

- **F-31 — `GET /api/v1/settings/export` discloses plaintext secrets.**
  `internal/launcher/settings.go:120-141` packs `INITIAL_ADMIN_PASSWORD`, `AI_API_KEY`, and
  `CONNECTOR_API_TOKEN` into `ExportedSettings`; `handlers/settings.go:37,93` JSON-encodes it
  straight into the HTTP response (`Content-Disposition: attachment`). In the documented
  Docker path, no `.env` file exists inside the container (`.dockerignore:29-31`), so the
  fallback branch (`handlers/settings.go:69-88`) runs and leaks "only" `AI_API_KEY` +
  `CONNECTOR_API_TOKEN` (**High** in that deployment). In the launcher/native path (binary
  running next to a real `.env`), the admin password leaks too (**Critical**). No audit trail
  of this endpoint being called exists (F-50).

### High

- **F-15 — CSRF fully bypassable via `Authorization: Bearer` prefix.**
  `internal/auth/csrf.go:75-81` skips the double-submit check the moment the `Authorization`
  header starts with `Bearer ` — **without validating that token** — while
  `internal/auth/middleware.go:22-33` accepts the `access_token` cookie as an equally valid
  alternative. The exemption's premise ("bearer requests are not ambient-credentialed") does
  not hold for this codebase. Chained with F-31, any script able to set one header value turns
  a logged-in admin's page visit into full credential disclosure.
- **F-20 — Rate-limit key is attacker-controlled.** `internal/server/router.go:89` installs
  `chi/middleware.RealIP`, explicitly marked deprecated/spoofable in its own source
  (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf). It honours client-supplied
  `True-Client-IP`/`X-Real-IP` and rewrites `r.RemoteAddr` before the careful trusted-peer
  logic in `internal/auth/limiter.go:229-257` ever runs. `Caddyfile:28-30` is a bare
  `reverse_proxy` that does not strip these headers. Rotating `True-Client-IP` per request
  defeats every rate limit in the codebase, including login lockout.
- **F-21 — `/auth/login` has no HTTP-level rate limit.** `router.go:253-255`: `/auth/refresh`
  and `/auth/logout` are covered, `/auth/login` is not. Combined with F-20, unlimited
  credential-stuffing / TOTP-guessing is possible, and each attempt costs the server a 64 MiB
  Argon2id derivation (concurrent-request memory-exhaustion DoS).
- **F-01 — TOTP secret stored in plaintext.** `internal/auth/service.go:289-293` writes the
  raw base32 secret into `totp_secret_encrypted` — the column name asserts encryption that
  does not exist; no crypto call anywhere in the auth path.
- **F-02 — No TOTP replay protection.** `internal/auth/totp.go:20-25` — a captured code is
  reusable for the entire time-step window (up to ~90s with default skew) across unlimited
  sessions; no last-used-step tracking exists.
- **F-32 — Settings-import response echoes submitted secrets back to the client** in plaintext
  JSON (`handlers/settings.go:131-135`).

### Medium (selection — full list of 52 findings available on request from the sweep transcript)

- **SSE broadcasts to every connected client.** `internal/sse/hub.go:29-40` has no per-user
  routing; `internal/core/notification/service.go:51-61` broadcasts another user's
  notification `title`/`message` to all connected sessions — cross-user data leak.
- **F-33 — `.env` injection via settings import.** `internal/launcher/config.go:250-294`
  builds `.env` lines with `fmt.Sprintf("KEY=%s", value)`, unescaped. A newline in any
  submitted field injects arbitrary environment variables (`DEMO_MODE`, `DATABASE_URL`,
  `CRM_SERVER_IMAGE`).
- **F-29 — SSRF via `OLLAMA_BASE_URL`/`AI_BASE_URL`.** The LLM gateway
  (`internal/ai/gateway.go:67-76`) uses a plain `http.Transport` with none of the
  DNS-rebinding-resistant guard the research scraper implements (`internal/ai/research.go:63-102`
  is exemplary and unaffected). The base URL is writable via settings-import and readable
  back through `/ai/chat` responses.
- **F-30 — Unsigned self-updater.** `internal/launcher/updater.go:492,731-776` downloads
  `refs/heads/main.zip` (a moving, unreleased branch) with no signature or checksum
  verification, then `chmod 0755` + `os.Rename` (`:402,407`) over the running binary,
  followed by `exec.Command` re-launch.
- **F-06/F-07 — Session invalidation gaps.** Access-token revocation is a process-local
  `sync.Map` (`internal/auth/service.go:50`), never swept, lost on restart; role/status
  changes (`handlers/users.go:183,248`) revoke nothing, so a demoted/deactivated user keeps a
  valid token for up to 30 minutes. The 10-second refresh-rotation grace cache
  (`service.go:217-224`) hands out the *new* token pair to whoever replays an already-consumed
  refresh token — no reuse detection.
- **F-04/F-05 — User enumeration.** The unknown-user timing mitigation
  (`service.go:138`) is a no-op: `CheckPassword` fails on base64 padding before ever calling
  `argon2.IDKey`, so the timing delta between "unknown user" and "wrong password" is fully
  observable. Deactivated/invited accounts get a distinct 403 with a literal message, checked
  *before* password verification and outside the failure-rate-limit counter.
- **F-09 — Ownership checks are fail-open.** Pattern `if claims != nil && claims.Role !=
  "ADMIN" { ... }` (deals.go:150,264; todos.go:139,211; notes.go:105,137;
  appointments.go:106,139) skips the entire authorization block if the context value is ever
  missing — contrast with the fail-closed `claims == nil ||` used in `backup.go:36`.
- **F-10/F-11 — No ownership enforcement on contacts/companies/notes.**
  `RequireAnyRole("ADMIN","VERTRIEB","BACKOFFICE","BENUTZER")` (`router.go:319,330,341,353,364,415`)
  lists every existing role and is therefore not a restriction at all — round-3 finding N17
  is only cosmetically closed. Note ownership compares `existing.Author != claims.Email` (a
  free-text string), not a stable user ID.
- **F-14 — Ungated routes:** `/emails/*`, `/telephony/calls`, `/automations` (GET),
  `/notifications/*`, `/ai/observability` carry no role check at all.
- **F-37 — CSP/HSTS exist only in the Caddyfile**, not at the Go layer (`router.go:94-101`
  sets only three headers) — any deployment without Caddy loses them silently.
- **F-38 — `Secure` cookie flag is conditional and fail-open** on client-settable
  `X-Forwarded-Proto` with `FORCE_SECURE_COOKIES=false` as the shipped default.
- **F-46 — No prompt-injection defence.** `internal/ai/guard.go` is PII-redaction only;
  `internal/ai/chat.go:64-69` concatenates client-supplied `m.Role` directly into the prompt
  transcript, allowing forged `SYSTEM:` turns; real CRM figures are interpolated into the same
  prompt (`chat.go:100-132`).
- **F-49/F-50/F-51 — Audit log gaps.** No hash chain, no append-only constraint,
  `ON DELETE SET NULL` on `user_id` (anonymizes history when a user is deleted). All 14
  call sites cover only business-entity CRUD — **no login, logout, password change, TOTP
  event, role change, settings export/import, backup export, or webhook failure is ever
  logged.** Every logged event has empty `ip_address`/`user_agent` (`"", ""` passed at every
  call site).
- **F-24/F-25 — Upload filtering is a blocklist**, not an allowlist
  (`internal/storage/local.go:21-33`) — `text/html`/`image/svg+xml` are permitted; the CAS
  dedup shortcut (`:92-101`) skips MIME validation entirely on a cache hit.
- **F-27 — Internal error strings echoed to clients** (`err.Error()` string-concatenated,
  unescaped, into JSON literals): `contacts.go:169`, `users.go:122` (leaks Postgres
  unique-violation text → e-mail enumeration), `deals.go:203`, `export.go:56`,
  `ai.go:90,107,128`, `settings.go:115,120,126`, `auth.go:171,296`, `notes.go:87`,
  `telephony.go:27`.
- **F-34 — Bootstrap admin password printed to logs** (`cmd/server/main.go:79-84`); no forced
  change on first login.
- **F-22 — Unauthenticated webhook** `/connectors/lead-intake` has no rate limit and no audit
  trail; in demo mode accepts the publicly-known `demo-connector-token` (`router.go:195-197`).
- **`WriteTimeout: 0`** set globally (`cmd/server/main.go:196`) to accommodate SSE — Slowloris
  exposure on every endpoint, not just the stream. No `http.MaxBytesReader` anywhere.
- **F-16/F-18/F-19 — CSRF token is unbound and has a predictable RNG fallback**
  (`hex.EncodeToString([]byte(time.Now().String()))`, `csrf.go:29-31`); `ClearCSRFCookie`
  omits `Secure`.
- **F-40 — `/api/v1/health` is unauthenticated** and discloses `demo_mode`, `ai_provider`,
  `ai_model`, and the internal `ai_base_url` (`router.go:104-116`).
- **Silent error-swallowing with fabricated success:** `core/reports/service.go:45-50`
  returns an empty report with `nil` error on DB failure; `core/export/service.go:107-144`
  `continue`s on every read error (unbounded loop on a persistent reader fault) and discards
  every `Create` error from CSV import rows.
- **Silent data loss via unguarded limits:** `Limit: 10000` in `core/export/service.go:41`,
  `core/reports/service.go:42`, `handlers/backup.go:42-45` — no warning surfaced anywhere.

### Explicitly sound (do not touch — call this out in the writeup so remediation doesn't regress it)

Ed25519 JWT with pinned algorithm (`internal/auth/jwt.go:45-50`); Argon2id at
m=64MiB/t=3/p=2 with `subtle.ConstantTimeCompare`; sqlc used throughout (zero dynamic SQL
construction anywhere); DNS-rebinding-resistant SSRF guard in the research scraper
(`internal/ai/research.go:63-102`); frontend has no `localStorage` token storage and no
`dangerouslySetInnerHTML`/`eval` sinks; non-root container (UID/GID 10001); Postgres not
port-mapped; `.env` never committed to git history; CSV export has working formula-injection
sanitization.

---

## P2 — Feature gaps, orphaned schema, test coverage, documentation drift

- **12 dead backend endpoints, never called by the frontend:** `POST /auth/refresh` (the
  frontend never refreshes — sessions die after 30 minutes despite a 30-day refresh token),
  `GET /{id}` for contacts/companies/deals/todos/notes, `POST /deals/{id}/solar-calculation`,
  `GET /emails/threads/{threadID}`, `GET /appointments/{id}/ics`, `POST /auth/totp/disable`,
  `POST /notifications/{id}/read`, `POST /automations/runs/{id}/approve`.
- **Orphaned tables:** `email_attachments` (sqlc code generated, zero callers, no upload
  endpoint), `company_research`, `intake_forms`, `workflow_steps` (steps actually live in
  `workflows.steps_json`/`workflow_runs.snapshot_json` instead).
- **`StorageService` is injected but never called** (`router.go:40`,
  `core/email/service.go:31,35`) — the README's "SHA-256-Deduplizierung" claim has no
  reachable code path.
- **⌘K search** covers only contacts/deals/companies (`CommandPalette.tsx:32,37,42`); README
  promises appointments and e-mails too.
- **Test coverage:** 65 Go test files, but `internal/ai/research.go` has zero tests,
  `internal/auth/middleware.go` and `csrf.go` have no direct tests, and
  `internal/queue/queue_test.go` only asserts the two `Kind()` strings. **No frontend unit
  test framework exists** — only Playwright. All 27 E2E tests run against the in-memory demo
  backend (`playwright.config.ts:24`, `DEMO_MODE=true`) so the real SQL paths never execute,
  and several assertions confirm exactly the fake UI paths listed above
  (`e2e/automations.spec.ts:32`, `e2e/inbox.spec.ts:34`,
  `e2e/calendar-and-telephony.spec.ts:8,15,32`) with pure `toBeVisible()` checks and no
  persistence round-trip.
- **Configuration drift:** four different hardcoded default model names
  (`router.go:83` `mistral`, `ai/gateway.go:55` `gemma4:12b`, `handlers/settings.go:66`,
  `ai/observability.go:161`) — `gemma4:12b` is not a real Ollama tag. The launcher writes
  `DB_USER=postgres`/`DB_HOST=db`/`STORAGE_PATH=/storage` (`launcher/config.go:250-294`) while
  `docker-compose.yml`/`.env.example` expect `crm_user`/`crm-db`/`/data/storage`.
  `docker-compose.yml` uses `DEMO_MODE=${DEMO_MODE:-false}`.
- **Documentation vaporware:** README lines 42, 49, 53, 77, 78, 80, 87, 91, 96, 97, 98, 101,
  102, 106, 111, 117 describe features that do not exist or exist only partially. CHANGELOG
  1.0.6 lines 19, 29, 30 claim fixes that are demonstrably absent from the current code (see
  the CHANGELOG-vs-code table below).
- `docs/AUDIT_REPORT_2026-09-21.html` remains tracked in git and contains real-looking demo
  credentials (round 1, still unresolved after three remediation rounds).

### CHANGELOG 1.0.6 claims vs. verified code state

| CHANGELOG claim | Line | Verified state |
|---|---|---|
| "Kalender-Export: Entfernung unverbundener Microsoft/Google-Push-Versprechen" | 30 | **Not removed.** `CalendarPage.tsx:207,210,245,328-330` still shows "M365 & Google OAuth verbunden" and the two provider sync labels; `e2e/calendar-and-telephony.spec.ts:8,15,32` still asserts on them. |
| "Wahrheitsgemäße UI: Falsche pgvector- und 384-dim-Behauptungen … ersetzt" | 29 | **Partially not removed.** `SettingsPage.tsx:1541` still renders "PostgreSQL 16 + pgvector"; no vector column or embedding code exists. |
| "Workflow-Engine: `ApproveStep` bricht bei Step-Fehlern ab, markiert Run als FAILED" | 19 | Backend fix appears correct, but is **unreachable in production**: the "Schritt freigeben" UI button (`AutomationsPage.tsx:278-290`) never calls the endpoint. |
| "Kontrollierter Failsafe-Modus … Simulations-Fallbacks" (v1.0.5, referenced as resolved risk in v1.0.6) | — | `simulateGemmaResponse` is transparent only via a server log line; the HTTP response to the caller is indistinguishable from a real answer (A1 above). |

---

## Priority order for remediation

1. **F-31 (Critical) + F-15 (High)** — the settings-export credential leak and the CSRF
   bypass that makes it exploitable cross-origin. Fix together.
2. **F-20 + F-21 (High)** — rate-limit key spoofing and the unthrottled login endpoint; both
   defeat the entire brute-force defence model at once.
3. **F-01 + F-02 (High)** — TOTP plaintext storage and replay protection.
4. **E1–E8** — the e-mail subsystem: either build real SMTP/IMAP or remove every UI claim
   about it; this is the single largest fake-success surface in the product.
5. **A1–A9, U1–U6** — AI/automation fake-success cluster (silent LLM fallback,
   note-synthesis fabrication, dead-code triggers, unreachable approve flow).
6. **C1–C4, D1–D3, B1–B6** — calendar/telephony/map fabrication, CRM data-truth gaps
   (consent/energy fields, §355 BGB), backup/invite/settings correctness.
7. **Remaining Medium/Low security findings** (F-06/F-07 session invalidation, F-09/F-10/F-11
   ownership, F-14 ungated routes, F-24/F-25 upload allowlist, F-27 error leakage, F-37/F-38
   transport headers, F-46 prompt injection, F-49/F-50/F-51 audit log).
8. **P2 documentation and test-coverage remediation** — once the above is fixed, so the docs
   and tests describe the real system rather than needing a fifth review round.

See `docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md` for the task-by-task
implementation plan and `docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md`
for the target architecture.
