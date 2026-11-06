# Security & Feature Review — Round 3 (2026-09-21)

> **Scope:** Follow-up review of `openlocalcrm` at HEAD `927f973` (v1.0.5), after round-1
> (`docs/REPO_REVIEW_2026-09-21.md`) and round-2 (`docs/REPO_REVIEW_2026-09-21_round2.md`)
> remediation. Focus per request: **security** and **feature correctness** (does a feature
> actually do what it claims to do), not doc/hygiene issues already covered twice.
> **Method:** 5 parallel independent audits — round-2 H1-H6 re-verification, round-2 H7-H12
> re-verification, round-2 M1-M14/Low re-verification, a fresh backend sweep (RBAC/IDOR,
> file upload, CSRF, session config, migrations, new endpoints), and a fresh frontend +
> feature-completeness sweep (client-side stubs, fabricated success states, localStorage
> secrets) — plus manual file:line verification of every claim before inclusion here.
> **Status:** Read-only audit. No code changed. For processing: fix HIGH first.

---

## Executive summary

Round 2's remediation commit (`927f973`) genuinely fixed the majority of what it claimed —
12 of 12 HIGH findings show real code change, 11 of 14 MEDIUM findings are fully fixed. But:

1. **The AI/automation subsystem is still the highest-risk area**, exactly as round 2 called
   out as the cross-cutting theme. `ParseBill` is still 100% fake. The automation engine's
   `WEBHOOK`/`SET_TAG`/`DRAFT_EMAIL` actions are still no-ops that report success. A new,
   more severe instance of the same pattern was found in `ApproveStep`: a failed action
   during human-approved workflow continuation is only logged, never surfaces, and the run
   is still marked `COMPLETED`.
2. **Two new HIGH-severity IDOR vulnerabilities**, not present in either prior round: any
   authenticated user (regardless of role) can modify or reassign **any other user's deal**
   by ID, with no ownership/assignment check — unlike every other entity type in the same
   codebase (todos, notes, appointments) which already enforce this correctly.
3. **Two new HIGH-severity silent-failure bugs** in `appointments.go` Delete and
   `automation.ApproveStep`, where a DB error is discarded and the caller is told the
   operation succeeded.
4. Two round-2 MEDIUM findings remain genuinely unfixed (M2 token revocation, M5 AI gateway
   silent fallback, M6 stub email workers) — same risk as before, not re-explained here beyond
   the confirmation.
5. **Three more previously-undocumented "feature lies about success" instances** found by a
   dedicated frontend sweep, extending round 2's cross-cutting theme beyond the AI/automation
   subsystem into contact import and calendar sync: CSV contact import never calls the
   backend at all (N12), the knowledge-base "pgvector vectorization" claim is entirely
   fabricated — no embedding code exists anywhere in the repo (N13), and calendar push to
   Microsoft 365/Google only flips a DB flag with no external API call (N14).
6. **Core CRM entities have no role-based access control at all** (N17): `contacts`,
   `companies`, `deals`, `todos`, `notes`, `appointments` routes carry no
   `RequireRole`/`RequireAnyRole`. Migration `00011` made `VERTRIEB`/`BACKOFFICE` real,
   assignable roles, but nothing in the router enforces them on the core data — combined with
   the deal IDOR (N1/N2), any authenticated user of any role has unrestricted CRUD over all
   CRM data.

---

## HIGH

| # | Area | Finding | Evidence | Fix |
|---|------|---------|----------|-----|
| N1 | Authorization / IDOR | `internal/server/handlers/deals.go:124-203` (`DealHandler.Update`) — any authenticated user, including non-admin `VERTRIEB`, can update (retitle, reassign, change stage/value) **any deal by ID**, no ownership/assignment check. Every comparable entity (`todos.go:138-143`, `notes.go:105-115`, `appointments.go:100-110`) already restricts non-admin mutation to records the caller owns/is assigned to — this handler is the odd one out. | Verified by dedicated RBAC/IDOR sub-audit, read full router.go + handler + service.go. | Add the same "non-ADMIN must own/be assigned the deal" check already used in `TodoHandler.Update`/`Delete`. |
| N2 | Authorization / IDOR | `internal/server/handlers/deals.go:230-280` (`DealHandler.AttachSolarCalculation`) — same root cause as N1: any authenticated user can attach/overwrite the solar calculation on any deal by ID. | Same sub-audit. | Same ownership check as N1. |
| N3 | Feature correctness / silent failure | `internal/server/handlers/appointments.go:145,148` (`Delete`) — handler discards `h.svc.Delete`'s error entirely; `Service.Delete` (`internal/core/appointment/service.go:331-338`) returns early on DB failure *before* removing the appointment from the in-memory cache, so on failure nothing is deleted anywhere — yet the handler unconditionally responds `{"success":true}`. | Verified by feature-correctness sub-audit. | Check the error, return 500 on failure. |
| N4 | Feature correctness / silent failure | `internal/core/automation/service.go:318-337` (`ApproveStep`) — a step-execution error during human-approved workflow continuation is only `log.Printf`'d (line 324), never surfaced; the loop continues, `run.Status` is still set to `"COMPLETED"` (line 335) and persisted via `UpdateWorkflowRunStatus` (line 344, itself `_, _ =` discarded, so even *that* write's own failure is invisible). A failed `CREATE_TASK`/`NOTIFY_USER` write during approval is reported to the caller as a completed workflow. | Manually re-verified, file read in full (lines 300-352 above). | Propagate the step error, mark the run `FAILED` on failure, check `UpdateWorkflowRunStatus`'s own error. |
| N5 | Feature correctness | `internal/core/automation/service.go:456-461` (`WEBHOOK` action) — still just logs the configured URL and returns `nil` if non-empty; **no HTTP request is ever issued**. The step/run is reported as executed regardless. This is a residual, unfixed part of round 2's H6 (v1.0.5 only fixed `CREATE_TASK`/`NOTIFY_USER`). | Manually re-verified (lines 456-461 above). | Actually issue the webhook call (with the same private-IP/SSRF guard used in `internal/ai/research.go` once implemented) and return its error. |
| N6 | Feature correctness | `internal/server/handlers/ai.go:146-173` (`ParseBill`) — confirmed **still fully unfixed** from round 2's H5: endpoint returns fully hardcoded fake data ("Familie Müller" etc.), does not accept or parse any uploaded file. Advertised as an AI feature, is pure mock. | Verified by feature-correctness sub-audit, cross-checked file content. | Implement real parsing or clearly mark endpoint as stub/disabled in API docs and UI. |
| N7 | Feature correctness | `internal/server/handlers/ai.go:249-274` (`DeleteKB`) — new beyond round 2's M9. M9's uuid-parse-failure case is now correctly fixed (400 on parse error). But when the id parses fine and `h.querier.DeleteKBArticle` itself fails (DB error), that error is discarded (line 258); handler still strips the item from the in-memory list and returns `{"success":true}` — the article can remain in the DB while the client is told it's gone. | Verified by feature-correctness sub-audit. | Check the error, return 500 instead of a false success. |
| — | AI / unconfirmed writes | `internal/ai/chat.go:377-430` (`handleActionableIntent`) — round 2's H3 is **still unfixed**: pure keyword/regex match on raw chat text (`"anlegen"`/`"erstellen"` + `"kunde"`/`"firma"`) still directly calls `CreateCompany` with no confirmation step, unlike the automation engine's explicit approval flow. Any chat message like *"Firma Mustermann GmbH als Kunden anlegen"* silently writes to the DB. (Round 2's H4 — the ignored error on this same call — **is fixed**: error is now checked at line 423 and a distinct error reply/`ActionCard` is returned.) | Manually re-read full function (lines 377-450). | Route through a confirm/`ActionCard` step before writing, matching the automation engine's `WAITING_APPROVAL` pattern. |
| N12 | Feature correctness | `web/src/components/contacts/CSVImportModal.tsx:56-67` (`handleExecuteImport`) — never calls the backend. Just a `setTimeout` followed by a hardcoded `"3 Kontakte importiert"` toast regardless of actual CSV content (even a 500-row file), then `onImportComplete()`. Users believe a GDPR-cited (§6.2) contact import succeeded; zero data is persisted. | Verified by frontend feature-completeness sweep. | Implement a real `POST` to a contacts-import endpoint and reflect the actual row/dedupe counts. |
| N13 | Feature correctness | `web/src/pages/AIAssistantPage.tsx:149` + `internal/server/handlers/ai.go:210-247` (`CreateKB`) — UI claims the article is "in pgvector vektorisiert (384-dim Embeddings gespeichert)"; backend `CreateKB` only inserts plain text (Title/Category/Content/Tags), `ChunksCount` is hardcoded to `4`, no embedding call exists anywhere in the codebase, and the `vector` extension (enabled in `internal/db/migrations/00001_initial_schema.sql:5`) is never used by any table/column. The advertised "semantic KB search" is entirely fabricated. | Verified by frontend feature-completeness sweep. | Implement real embedding generation + pgvector column/search, or stop claiming vectorization in the UI/docs. |
| N14 | Feature correctness | `web/src/pages/CalendarPage.tsx:129-142` + `internal/core/appointment/service.go:353-370` (`PushExternal`) — UI claims the appointment was "erfolgreich in Ihren Microsoft 365 & Google Kalender übertragen"; backend `PushExternal` only flips a DB boolean `IsPushed=true` — no Microsoft/Google Calendar API integration exists anywhere in the repo. Distinct from N10 (which is about the discarded DB-write error on the same function) — this is the fabricated external-sync claim itself. | Verified by frontend feature-completeness sweep. | Implement a real calendar-provider push or remove the claim from the UI. |
| N17 | Authorization | `internal/server/router.go` — `/api/v1/contacts`, `/companies`, `/deals`, `/todos`, `/notes`, `/appointments` carry **no** `RequireRole`/`RequireAnyRole` at all; any authenticated user of any role gets full CRUD (including delete) over all org-wide CRM data. Migration `00011_extend_user_roles.sql` made `VERTRIEB`/`BACKOFFICE` real, assignable roles, but the router never checks them on core CRM routes — the role model now exists in the DB with zero enforcement on the data it's supposed to gate. Combined with the deal IDOR (N1/N2), effectively any logged-in account is equivalent to admin for CRM data (short of the `ADMIN`-only user/backup/settings routes, which are correctly gated). | Verified by backend RBAC sweep, full `router.go` read. | Decide the intended role-scoping model (e.g. `VERTRIEB` limited to own/assigned records, `BACKOFFICE` read-mostly) and enforce it with `RequireAnyRole`/ownership checks consistent with `todos.go`/`notes.go`. |

## MEDIUM

| # | Area | Finding | Status vs. round 2 | Fix |
|---|------|---------|---------------------|-----|
| N8 | Authorization | `internal/server/handlers/notifications.go:45-60` (`MarkRead`) + `internal/core/notification/service.go:80-82` (`MarkAsRead`) — takes only the notification `id`, never the caller's user ID; any authenticated user can mark any other user's notification as read by guessing/enumerating its UUID (`UPDATE ... WHERE id = $1`, no `user_id` predicate). New finding, not in round 2. | New | Pass the caller's `userID` into `MarkAsRead`, scope `WHERE id = $1 AND user_id = $2`. |
| N9 | Feature correctness | `internal/core/automation/service.go:357-374` (`SET_TAG`) and `:411-428` (`DRAFT_EMAIL`) — v1.0.5's fix only added an audit-log write; `SET_TAG` never mutates the target's actual tag, `DRAFT_EMAIL` never persists a retrievable draft, and even that audit-log write is `_, _ =` discarded. `executeStep` returns `nil` (success) regardless of what actually happened. | Residual from H6 | Implement the real side effect; check `CreateAuditLog`'s error. |
| N10 | Feature correctness | `internal/core/appointment/service.go:353-361` (`PushExternal`) — discards `UpdateAppointmentPushStatus`'s error, still flips in-memory `IsPushed=true` and returns `nil`; handler reports `{"status":"synced"}` even when the DB write failed. | New | Propagate/check the error. |
| N11 | Backend/AI | `internal/server/handlers/ai.go:316-379` (`CreateResearchJob`) — round 2's M4 is **partially fixed**: now calls `researchSvc.ResearchCompany` when available and overwrites fields on success, but on nil service or research error it silently falls back to the same fabricated boilerplate as before and still reports the job `Status: "COMPLETED"`. | Partial | On research failure, mark the job `FAILED`, don't fabricate a "completed" result. |
| M2 (confirmed) | Security | `internal/auth/service.go:50` — access-token revocation is still a pure in-memory `sync.Map`, no DB/Redis persistence found anywhere in the repo. Lost on restart/multi-replica exactly as round 2 described. | Still broken | Persist revocations or shorten access-token TTL (round 2's original recommendation stands). |
| M5 (confirmed) | Backend/AI | `internal/ai/gateway.go:146-148,188-190` — `callOpenAI`/`callOllama` still silently fall back to `simulateGemmaResponse` on any error/non-200; only a server-side log line (`[AI_GATEWAY_NOTICE]`) distinguishes it, response to the caller is unchanged. | Still broken | Surface a distinct error/degraded-mode signal to the caller instead of silent fallback text. |
| M6 (confirmed) | Backend | `internal/queue/jobs.go:23-39,53-68` — `EmailSyncWorker`/`EmailTriageWorker` remain explicit no-op stubs ("demo mode this logs the action" per in-code comment) registered as real jobs. | Still broken | Implement or remove from the job registry until implemented. |
| M12 (confirmed) | Frontend | `web/src/pages/SettingsPage.tsx:368-422` (`handleImportSettings`) — round 2's M12 is **partially fixed**: now validates root-is-object and type-checks `ai.provider`/`ai.model`/`admin.email`, but `backup` and other nested fields remain unchecked, and the full raw parsed object (including unvalidated extra keys) is still POSTed as-is. | Partial | Validate the full expected shape, or strip unknown/unvalidated keys before POSTing. |
| N15 | Feature correctness | `web/src/pages/CalendarPage.tsx:144-153,185-191` (`handleSyncExternal`, `handleSendInviteEmail`) — pure client-side stubs, no `apiFetch` call at all; `handleSendInviteEmail` tells the user an ICS invite was sent to the customer's real e-mail address when nothing was sent. | New | Wire to real endpoints or remove the UI affordance. |
| N16 | Security / secrets-in-browser | `web/src/pages/SettingsPage.tsx:741-747` — the real `connector_api_token` (used to authenticate inbound webhook leads, i.e. a live bearer secret, not a display placeholder) is written to `localStorage` in plaintext on every keystroke. Readable by any XSS payload or malicious browser extension on the origin; worse than round 2's H10 (which only flagged a hardcoded *display* value, not a persisted live secret). | New | Keep the token server-side only; mask it in the UI, don't persist to `localStorage`. |

## Verified FIXED (round 2 → round 3, no action needed)

- **H1** (JWT in `localStorage`): fixed — `web/src/context/AuthContext.tsx`/`web/src/api/client.ts` no longer touch `localStorage` for the auth token (grep clean). A separate `connector_api_token` is still written to `localStorage` in `SettingsPage.tsx` — see **N16** below, this is a live secret, not benign.
- **H2** (`.dockerignore` secrets leak): fixed — now mirrors `.gitignore`, excludes `.env*`, `*.key`, `*.pem`, `data/`, `backups/`, `*.sql` (with migration/query exceptions).
- **H4** (ignored `CreateCompany` error in chat): fixed — error now checked, distinct error reply returned.
- **H7** (unbounded appointments list): fixed — pagination added.
- **H8/H9** (backup drill ignores errors / hardcoded 10k limit): fixed per M1-M14 sub-audit.
- **H10/H11/H12** (fake connector token, fake webhook test, fake 2FA QR code): fixed per M1-M14 sub-audit (not re-verified line-by-line in this pass beyond the sub-audit's confirmation).
- **M1** (`Secure: r.TLS != nil` cookie bug): fixed — `internal/server/handlers/auth.go:16-27` now derives secure-cookie status from `X-Forwarded-Proto`/`FORCE_SECURE_COOKIES`, acceptable given M3 also closed direct port exposure.
- **M3** (spoofable `X-Forwarded-For`, exposed demo port): fixed — `internal/auth/limiter.go:236-257` only trusts forwarded headers from a loopback/private/link-local peer; `docker-compose.demo.yml` no longer publishes port 8080 directly.
- **M7/M8** (deal pagination): fixed.
- **M9** (`DeleteKB` false success on bad UUID): fixed (see N7 above for the *narrower* residual DB-error case, which is new).
- **M10** (rate-limiter goroutine leak / duplicate instance): fixed — single shared limiter with `Stop()` wired to context cancellation.
- **M11** (dual auth transport): fixed, folded into H1 fix.
- **M13** (hardcoded Go version in UI): fixed — now reads "Go 1.26".
- **M14** (env var doc/`.env.example`/code reconciliation): fixed.
- Low: `deals.go:170` malformed numeric now returns 400 instead of silently ignoring.
- Low: `connectors/connector.go` `IngestLead` — failed deal creation is no longer silently dropped, now logs and returns a real error (bad numeric input is still zero-filled rather than rejected — low-severity residual, not re-listed as a separate finding).
- Low: basic security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`) now set at the Go app layer, not just Caddy (CSP/HSTS remain Caddy-only, unchanged).
- No new injection, deserialization, SSRF, or file-upload vulnerabilities found (dedicated sub-audit: CSV formula-injection already sanitized and tested; storage layer unchanged/safe and currently unreachable from any handler; no `yaml.Unmarshal`/`xml.Unmarshal` in the repo; AI gateway output guard still strips script/leak signatures; no open-redirect surface in frontend).
- No hardcoded secrets/API keys found; no CI/CD workflow exposes secrets to fork-PR-triggered runs; Dockerfiles run as non-root `crmuser` (10001:10001); Ed25519 key loading fails closed (fatal error) outside demo mode rather than falling back to an ephemeral key.
- `users.go`/`backup.go`/`settings.go`/`automations.go` (`CreateWorkflow`/`ApproveStep`)/`export.go` — all correctly `RequireRole("ADMIN")`-gated at the router, re-checked in-handler where relevant; last-admin-protection logic sound.

## Noted but out of scope (already tracked, not re-detailed here)

- `docs/AUDIT_REPORT_2026-09-21.html` still tracked in git with real-looking demo credentials (`demo123`) and the weak `crm_pass` DB-password reference (round 1, unresolved).
- Weak `crm_pass` fallback in `cmd/server/main.go`'s `dbURL` default and `internal/launcher/templates.go` for non-compose/local runs (round 1, unresolved; not exploitable via the documented Docker deployment path since `docker-compose.yml:76` enforces `DB_PASSWORD` via `:?`).
- Role enum functional mismatch: DB now has `ADMIN`/`VERTRIEB`/`BACKOFFICE` (`internal/db/migrations/00011_extend_user_roles.sql`), but `users.go` handler validation still only accepts `ADMIN`/`BENUTZER` for invite/role-update — a functional bug, not an authorization vulnerability (no privilege escalation path), flagged for engineering follow-up.

---

## Priority order for remediation

1. **N1, N2, N17** — deal IDOR + no role enforcement at all on core CRM routes; any
   authenticated user of any role has unrestricted CRUD over all CRM data.
2. **N3, N4** — silent-failure "false success" bugs (appointment delete, automation approve).
3. **N5, N6, N12, N13, N14** — features that fabricate success/results with no real backend
   work: automation `WEBHOOK`, `ParseBill`, CSV contact import, KB "vectorization" claim,
   calendar push-to-external claim.
4. **H3 (chat.go unconfirmed write)** — add confirmation gate to `handleActionableIntent`.
5. **N7, N8** — narrower silent-failure/IDOR bugs (KB delete DB error, notification mark-read).
6. **N9, N10, N11, N15** — remaining automation/appointment/calendar-sync/research-job
   correctness gaps.
7. **N16** — live connector secret persisted in plaintext `localStorage`.
8. **M2, M5, M6, M12** — confirmed-still-open round-2 items.
