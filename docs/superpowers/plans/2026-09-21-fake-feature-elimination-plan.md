# Round-4 Remediation — Fake Feature Elimination & Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Vollständige Beseitigung aller simulierten/gefakten Features (P0), Behebung aller 52 Sicherheitsbefunde (P1) und Schließung von Feature-Lücken/Docs-Drift (P2) über 9 sequentielle Teilprojekte (SP1 bis SP9) gemäß dem genehmigten Design-Dokument `docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md`.

**Architecture:** Konsequentes Zero-Fake-Success-Prinzip: Echter SMTP-Versand und IMAP-Sync mit UID-Tracking; echtes pgvector-Embedding (768-dim `nomic-embed-text`); lokaler Kalender mit RFC 5545 `.ics`-Export (Entfernung von M365/Google Graph-Push); echte Event-Verdrahtung der Automation-Engine; AES-256-GCM-Secrets-Verschlüsselung; strikte CSRF-, RBAC- und Rate-Limiting-Härtung; Audit-Log-Hashkette.

**Tech Stack:** Go 1.26, Chi v5, PostgreSQL 16 (`pgvector`, `pg_trgm`), `jackc/pgx/v5`, sqlc, River Queue, Argon2id, `emersion/go-imap/v2`, `emersion/go-message/mail`, stdlib `net/smtp`, React 18, TypeScript, Tailwind CSS, Vite 6, Vitest, Playwright.

## Global Constraints
- Alle Go-Tests müssen mit `go test ./...` fehlerfrei durchlaufen; Coverage-Gate: `scripts/coverage.sh --min 50.0 --diff 50.0` (≥ 50% Diff-Coverage pro PR).
- Definition of Done je SP: `make check` grün (gofmt, go vet, web lint, web typecheck, license check, unit tests).
- Keine Scheinerfolge: Externe Abhängigkeiten schlagen ehrlich fehl (HTTP 502/503), niemals `err == nil` mit Dummy-Antworten.
- Zwingendes KI-Backend: Ollama oder konfigurierter Provider erforderlich; auch im Demo-Modus muss eine echte KI angeschlossen sein.
- RAM-Budget: `< 420 MB` Ruhezustand-Ziel (`crm-server`), `2 GB` transiente Burst-Obergrenze für Index-Builds, Backup-Streams und 4 parallele Argon2id-Ableitungen.
- TOTP ist rein optional / Opt-in für Nutzer; blockiert keine regulären Logins.
- Unveränderliche Invarianten: `migrations`-Symlink bleibt intakt; Migrationen sequentiell `NNNNN_` mit LF; SQL-Änderungen über `queries/*.sql` + `sqlc generate`.

---

## Übersicht der 9 Sub-Projekte

| SP | Titel | Branch | Findings |
|---|---|---|---|
| **SP1** | Security Critical/High & Login-Härtung | `fix/sp1-security-critical` | F-31, F-15, F-20, F-21, F-32, F-33, F-04, F-05 (Opt-in: F-01, F-02) |
| **SP2** | Real E-Mail (SMTP / IMAP) | `fix/sp2-real-email` | E1–E8, F-36 |
| **SP3** | AI Truthfulness & Embeddings | `fix/sp3-ai-truth` | A1–A9 |
| **SP4** | Automation Engine | `fix/sp4-automations` | U1–U6 |
| **SP5** | Kalender, Telefonie & Geocoding | `fix/sp5-calendar-telephony-map` | C1–C4, F-12, F-13 |
| **SP6** | CRM Data Truth & UWG/BGB | `fix/sp6-crm-data-truth` | D1–D3, F-52 |
| **SP7** | Backup / Restore, Invites & Settings | `fix/sp7-backup-settings` | B1–B6 |
| **SP8** | Verbleibende Sicherheit (SP8a/b/c) | `fix/sp8-security-hardening` | F-03, F-06..F-11, F-14..F-19, F-22..F-30, F-34, F-35, F-37..F-45, F-47..F-51 |
| **SP9** | Docs, Tests, Dead Endpoints & Simulation | `fix/sp9-truth-and-tests` | P2-Liste, S1–S5 |

---

# Phase 1: SP1 — Security Critical/High & Login-Härtung

### Task 1.1: Secrets-Redaction in Settings-Export/Import (F-31, F-32, F-33)

**Files:**
- Create: `internal/crypto/crypto.go`
- Create: `internal/crypto/crypto_test.go`
- Modify: `internal/launcher/settings.go:120-155`
- Modify: `internal/server/handlers/settings.go:35-140`
- Modify: `internal/launcher/config.go:250-295`
- Test: `internal/server/handlers/settings_test.go`

**Interfaces:**
- Consumes: `SECRETS_KEY_PATH`
- Produces: `crypto.Encrypt(plaintext []byte) (string, error)`, `crypto.Decrypt(ciphertext string) ([]byte, error)`, `RedactedExportedSettings`

- [ ] **Step 1: Failing Test für Secrets-Redaction und `.env`-Injection schreiben**
  In `internal/server/handlers/settings_test.go` testen:
  - `TestExportSettings_NeverOutputsPlaintextSecrets`: Admin-Export enthält keine Passwörter oder Tokens im Klartext, sondern nur `has_<secret>: true`.
  - `TestImportSettings_RejectsNewlines`: JSON-Felder mit `\n` oder `\r` führen zu HTTP 400.
  - `TestImportSettings_DoesNotEchoSecrets`: Antwort enthält keine Passwörter im Klartext.

- [ ] **Step 2: Tests ausführen und Fehlschlag verifizieren**
  Befehl: `go test -v -run "TestExportSettings_NeverOutputsPlaintextSecrets|TestImportSettings_RejectsNewlines" ./internal/server/handlers/...`
  Erwartung: FAIL (Export leakt aktuell Passwörter, Newlines werden unescaped in `.env` geschrieben).

- [ ] **Step 3: Implementierung `internal/crypto` und Redaction in Handlern**
  1. `internal/crypto/crypto.go`: AES-256-GCM mit automatischem Generieren eines 32-Byte-Schlüssels unter `SECRETS_KEY_PATH` (Berechtigung 0600), falls Datei nicht existiert.
  2. `internal/server/handlers/settings.go`:
     - `ExportSettings`: Passwörter und API-Keys mit Redaction-Helper auf Maske setzen.
     - `ImportSettings`: Validierung gegen Steuerzeichen (`\r`, `\n`) in Keys und Values. Antwort redigieren.
  3. `internal/launcher/config.go`: Sicheres Quoting bei `.env`-Generierung.

- [ ] **Step 4: Tests ausführen und Pass verifizieren**
  Befehl: `go test -v -run "TestExportSettings|TestImportSettings" ./internal/server/handlers/...`
  Erwartung: PASS.

- [ ] **Step 5: Commit**
  ```bash
  git add internal/crypto internal/launcher internal/server/handlers
  git commit -m "fix(security): redact secrets in settings export/import and prevent env injection (F-31, F-32, F-33)"
  ```

---

### Task 1.2: CSRF-Bypass via Bearer-Header beheben (F-15)

**Files:**
- Modify: `internal/server/csrf.go`
- Modify: `internal/auth/csrf.go`
- Modify: `internal/auth/middleware.go`
- Modify: `internal/server/router.go`
- Test: `internal/server/csrf_test.go`
- Test: `internal/auth/csrf_test.go`

**Interfaces:**
- Consumes: Bearer-Header im Request
- Produces: `auth_via_bearer` Context-Key nur nach echter kryptographischer JWT-Prüfung; Cookie-basierte Mutationen erfordern immer CSRF-Header.

- [ ] **Step 1: Failing Test für Bearer-Bypass schreiben**
  In `internal/server/csrf_test.go`:
  - `TestCSRF_InvalidBearerHeaderDoesNotBypassDoubleSubmit`: Ein Request mit Cookie und einem ungültigen `Authorization: Bearer invalid` wird mit 403 abgewiesen, wenn das CSRF-Token fehlt.
  - `TestCSRF_ConnectorWebhookExempt`: `/connectors/lead-intake` funktioniert mit validem Connector-Token außerhalb der CSRF-Gruppe.

- [ ] **Step 2: Test ausführen und Fehlschlag verifizieren**
  Befehl: `go test -v -run "TestCSRF_InvalidBearerHeaderDoesNotBypassDoubleSubmit" ./internal/server`
  Erwartung: FAIL (bisher wird CSRF übersprungen, sobald der String mit `Bearer ` beginnt).

- [ ] **Step 3: CSRF-Schutz fail-closed umbauen**
  1. In `internal/auth/middleware.go`: Setze `auth_via_bearer = true` im Context **nur**, wenn das Token über `ValidateAccessToken` erfolgreich validiert wurde und die Authentifizierung nicht über das Cookie erfolgte.
  2. In `internal/auth/csrf.go` und `internal/server/csrf.go`:
     - Überspringe die Double-Submit-Prüfung nur, wenn `auth_via_bearer == true`.
     - Entferne den unsicheren Fallback `hex(time.Now().String())`; bei RNG-Fehlern sofort HTTP 500.
     - `ClearCSRFCookie` setzt `Secure: true`.
  3. In `internal/server/router.go`:
     - `/connectors/lead-intake` explizit außerhalb der CSRF-Middleware-Gruppe mit eigener Token-Authentifizierung mounten.

- [ ] **Step 4: Tests ausführen und Pass verifizieren**
  Befehl: `go test -v ./internal/auth ./internal/server`
  Erwartung: PASS.

- [ ] **Step 5: Commit**
  ```bash
  git add internal/auth internal/server
  git commit -m "fix(security): make CSRF verification fail-closed and fix bearer bypass (F-15)"
  ```

---

### Task 1.3: Rate-Limiting-Härtung & Login-Schutz (F-20, F-21, F-04, F-05)

**Files:**
- Modify: `internal/server/router.go:85-110, 250-265`
- Modify: `internal/auth/limiter.go`
- Modify: `internal/auth/service.go:130-170`
- Modify: `Caddyfile:25-35`
- Test: `internal/auth/limiter_test.go`
- Test: `internal/auth/service_test.go`

**Interfaces:**
- Consumes: `r.RemoteAddr`, Login-Requests
- Produces: `LoginLimiter` mit 5 Versuchen/Min/IP+Email, Semaphore für max. 4 parallele Argon2id-Hashes, Constant-Time Dummy-Hash für unbekannte Nutzer.

- [ ] **Step 1: Failing Tests für Rate-Limiting & Timing schreiben**
  In `internal/auth/limiter_test.go` und `internal/auth/service_test.go`:
  - `TestLoginRateLimit_TriggeredAfter5Attempts`: Der 6. Versuch liefert 429.
  - `TestRealIP_HeaderSpoofingIgnored`: Gefälschte `True-Client-IP` Header verändern den Rate-Limit-Key nicht, wenn sie nicht von einem vertrauenswürdigen Proxy stammen.
  - `TestLogin_ConstantTimeOnUnknownUser`: Dauer eines falschen Passworts und eines unbekannten Benutzers unterscheiden sich nicht messbar (beide führen Argon2id aus).

- [ ] **Step 2: Tests ausführen und Fehlschlag verifizieren**
  Befehl: `go test -v -run "TestLoginRateLimit|TestRealIP_HeaderSpoofing|TestLogin_ConstantTime" ./internal/auth/...`
  Erwartung: FAIL.

- [ ] **Step 3: Implementierung Rate-Limiter, Semaphor und Dummy-Hash**
  1. In `router.go`: Entferne `chi/middleware.RealIP`. Installiere vertrauenswürdige IP-Extraktion basierend auf `RemoteAddr` und konfigurierter Reverse-Proxy-CIDR.
  2. Im `Caddyfile`: Strippe client-seitige `True-Client-IP` und `X-Real-IP`.
  3. In `internal/auth/service.go`:
     - Initialisiere Semaphor: `argon2Sem = make(chan struct{}, 4)`.
     - Bei unbekanntem Benutzer: Berechne Dummy-Argon2id-Hash mit identischen Parametern (`m=65536, t=3, p=2`), bevor `ErrInvalidCredentials` geworfen wird.
     - Deaktivierte Konten werfen dieselbe generische Fehlermeldung wie falsche Passwörter.
  4. In `internal/server/router.go`: Registriere `LoginLimiter` auf `/auth/login`.

- [ ] **Step 4: Tests ausführen und Pass verifizieren**
  Befehl: `go test -v ./internal/auth/... ./internal/server/...`
  Erwartung: PASS.

- [ ] **Step 5: Commit**
  ```bash
  git add internal/auth internal/server Caddyfile
  git commit -m "fix(security): rate-limit login, protect argon2id with semaphore, and mitigate user enumeration (F-20, F-21, F-04, F-05)"
  ```

---

### Task 1.4: Opt-In TOTP Verschlüsselung & Replay-Schutz (F-01, F-02)

**Files:**
- Create: `internal/db/migrations/00007_totp_hardening.sql`
- Symlink: `migrations` (automatisch via Symlink)
- Modify: `internal/db/queries/users.sql`
- Modify: `internal/auth/service.go`
- Modify: `internal/auth/totp.go`
- Test: `internal/auth/totp_test.go`

- [ ] **Step 1: Migration für `totp_last_used_step` und `totp_secret_encrypted_at` anlegen**
  Erstelle `internal/db/migrations/00007_totp_hardening.sql` (LF Endings):
  ```sql
  ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_used_step BIGINT DEFAULT NULL;
  ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret_encrypted_at TIMESTAMPTZ DEFAULT NULL;
  ```
  Führe `sqlc generate` aus.

- [ ] **Step 2: Failing Test für Replay-Schutz und Verschlüsselung schreiben**
  In `internal/auth/totp_test.go`:
  - `TestVerifyTOTP_ReplayBlocked`: Ein bereits verwendeter Code im selben Zeitfenster wird abgewiesen.
  - `TestTOTPSecret_StoredEncrypted`: In der DB steht kein Base32-Plaintext.

- [ ] **Step 3: Implementierung von Verschlüsselung und Replay-Check**
  1. Nutze `internal/crypto` zum Ver- und Entschlüsseln des TOTP-Secrets.
  2. Bei erfolgreicher Verifikation: Prüfe `step > totp_last_used_step` und aktualisiere `totp_last_used_step = step`.

- [ ] **Step 4: Tests ausführen und Pass verifizieren**
  Befehl: `go test -v ./internal/auth/...`
  Erwartung: PASS.

- [ ] **Step 5: Commit**
  ```bash
  git add internal/db internal/auth
  git commit -m "fix(security): encrypt stored TOTP secrets and prevent token replay (F-01, F-02)"
  ```

---

# Phase 2: SP2 — Real E-Mail (SMTP / IMAP)

### Task 2.1: Mailclient-Treiber & DB-Erweiterung (E1–E8, F-36)

**Files:**
- Create: `internal/db/migrations/00008_email_accounts_and_tags.sql`
- Create: `internal/mailclient/smtp.go`
- Create: `internal/mailclient/imap.go`
- Create: `internal/mailclient/mailclient_test.go`
- Modify: `internal/db/queries/emails.sql`
- Modify: `internal/core/email/service.go`
- Test: `internal/core/email/service_test.go`

**Dependencies:**
- `github.com/emersion/go-imap/v2` (MIT)
- `github.com/emersion/go-message/mail` (MIT)

- [ ] **Step 1: Go-Dependencies installieren & Lizenz-Check ausführen**
  ```bash
  go get github.com/emersion/go-imap/v2@latest
  go get github.com/emersion/go-message/mail@latest
  ./scripts/check-licenses.sh
  ```

- [ ] **Step 2: Migration für Email-Accounts & Message-Tags erstellen**
  In `internal/db/migrations/00008_email_accounts_and_tags.sql`:
  ```sql
  ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'personal';
  ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
  ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ;
  ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS last_uid BIGINT DEFAULT 0;
  ALTER TABLE email_messages ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;
  ```
  Führe `sqlc generate` aus.

- [ ] **Step 3: SMTP-Sender und IMAP-Fetcher mit in-test Fake-Servern testen**
  In `internal/mailclient/mailclient_test.go`:
  - Erstelle Fake-SMTP-Server über `net.Listen("tcp", "127.0.0.1:0")`.
  - Teste `SendMail`: Plaintext & Multipart-E-Mail wird fehlerfrei übertragen.
  - Teste `FetchNewMessages`: UID-basiertes Abholen parst Absender, Betreff, Body und Anhänge.

- [ ] **Step 4: `SMTPSender` und `IMAPFetcher` implementieren**
  1. `internal/mailclient/smtp.go`: Nutzt `net/smtp` mit TLS/STARTTLS.
  2. `internal/mailclient/imap.go`: Nutzt `emersion/go-imap/v2` und `go-message/mail`.
  3. `internal/core/email/service.go`: Verschlüsselt Passwörter mit `internal/crypto`. CRUD für Accounts + `TestConnection`-Funktion.

- [ ] **Step 5: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/mailclient/... ./internal/core/email/...
  git add go.mod go.sum internal/db internal/mailclient internal/core/email
  git commit -m "feat(email): add real SMTP sender and IMAP fetcher with encrypted credentials (E1, E6, E8, F-36)"
  ```

---

### Task 2.2: E-Mail API-Endpoints & River-Sync-Worker (E1, E4, E6, E7)

**Files:**
- Modify: `internal/server/handlers/emails.go`
- Modify: `internal/server/router.go:373-380`
- Modify: `internal/queue/jobs.go:20-75`
- Test: `internal/server/handlers/emails_test.go`
- Test: `internal/queue/queue_test.go`

- [ ] **Step 1: Failing Tests für E-Mail Endpoints & River-Jobs schreiben**
  - `POST /api/v1/emails/send`: Versendet echte E-Mail, speichert Outbound-Message.
  - `POST /api/v1/emails/accounts/{id}/test`: Führt Live-Check gegen IMAP & SMTP durch.
  - `EmailSyncWorker`: Holt Nachrichten über den Mailclient ab und speichert sie in der DB.

- [ ] **Step 2: Endpoints und Worker verdrahten**
  1. In `handlers/emails.go`: Handlers für `SendEmail`, `CreateAccount`, `ListAccounts`, `TestAccountConnection`, `TagMessage`.
  2. In `queue/jobs.go`: `EmailSyncWorker` führt `FetchNewMessages` aus, weist Kontakte zu und reiht bei neuen Mails den `EmailTriageWorker` ein.
  3. In `router.go`: Alle E-Mail-Routen in `protected.Route("/emails")` registrieren.

- [ ] **Step 3: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/server/handlers/... ./internal/queue/...
  git add internal/server/handlers internal/server/router.go internal/queue
  git commit -m "feat(email): implement real email sending, account testing, and river sync worker (E1, E4, E6, E7)"
  ```

---

### Task 2.3: Frontend Inbox & E-Mail-Einstellungen anbinden (E1–E5)

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/pages/InboxPage.tsx`
- Modify: `web/src/pages/SettingsPage.tsx`
- Test: `web/src/pages/InboxPage.test.tsx` (oder E2E-Spec)

- [ ] **Step 1: API-Client um E-Mail-Funktionen erweitern**
  In `web/src/api/client.ts`:
  - `sendEmail(payload)`
  - `getEmailAccounts()`, `createEmailAccount()`, `testEmailAccount(id)`
  - `tagEmailMessage(id, tags)`

- [ ] **Step 2: `InboxPage.tsx` von Fake auf Real umstellen**
  - `handleSendReply`: Ruft `api.sendEmail` auf. Bei Fehler: Ehrlicher Toast.
  - KI-Antwortentwurf: Ruft echten `POST /api/v1/ai/triage` auf statt `setTimeout` mit Dummy-Text.
  - Tags: Persistieren über `api.tagEmailMessage` statt lokalem `useState`.

- [ ] **Step 3: `SettingsPage.tsx` E-Mail-Tab aktivieren**
  - Statisches JSX durch echtes Account-Management (Liste, Neues Konto, Verbindung testen) ersetzen.

- [ ] **Step 4: Frontend-Build & Lint prüfen**
  ```bash
  cd web && pnpm lint && pnpm typecheck && pnpm build && cd ..
  ```

- [ ] **Step 5: Commit**
  ```bash
  git add web/
  git commit -m "feat(web): wire inbox and settings to real email backend endpoints (E1, E2, E3, E4, E5)"
  ```

---

# Phase 3: SP3 — AI Truthfulness & Embeddings

### Task 3.1: Fakes aus Gateway & Chat entfernen (A1, A2, A3, A4, A5)

**Files:**
- Modify: `internal/ai/gateway.go:140-250`
- Modify: `internal/ai/chat.go:150-320`
- Modify: `internal/server/handlers/notes.go:155-190`
- Modify: `internal/server/handlers/ai.go:190-215`
- Test: `internal/ai/gateway_test.go`
- Test: `internal/ai/chat_test.go`
- Test: `internal/server/handlers/notes_test.go`

- [ ] **Step 1: Failing Tests für striktes Fehlerverhalten schreiben**
  - Wenn Ollama/Provider 500 liefert: Gateway gibt Fehler zurück, kein Canned-Text.
  - `POST /api/v1/ai/synthesize-notes`: Liest Notizen real ein, ruft Gateway auf; bei Gateway-Fehler → HTTP 502.
  - `POST /api/v1/ai/parse-bill`: Erfordert `document_text`, kein „Familie Müller“-Mock.

- [ ] **Step 2: Dummy-Funktionen löschen**
  1. In `gateway.go`: `simulateGemmaResponse` komplett löschen. Response-Body bei Fehlern ordnungsgemäß mit `defer resp.Body.Close()` schließen.
  2. In `chat.go`: `generateSmartFallback` und `isGenericSimulatedReply` löschen. Bei LLM-Fehler ehrliches `ErrUpstreamUnavailable` zurückgeben. `researchSvc` in `ChatService` injecten.
  3. In `handlers/notes.go`: `SynthesizeNotes` liest Request-Notizen, formuliert Prompt und ruft `gateway.Generate` auf.

- [ ] **Step 3: Tests ausführen & Pass verifizieren**
  Befehl: `go test -v ./internal/ai/... ./internal/server/handlers/...`
  Erwartung: PASS.

- [ ] **Step 4: Commit**
  ```bash
  git add internal/ai internal/server/handlers
  git commit -m "fix(ai): eliminate simulated responses and fake note synthesis (A1, A2, A3, A4, A5)"
  ```

---

### Task 3.2: Echte pgvector-Embeddings & Semantische Suche (A9)

**Files:**
- Create: `internal/db/migrations/00009_pgvector_768.sql`
- Modify: `internal/ai/gateway.go` (Embedding-Funktion)
- Modify: `internal/db/queries/ai.sql`
- Modify: `internal/server/handlers/ai.go`
- Test: `internal/ai/embeddings_test.go`

- [ ] **Step 1: Migration für `vector(768)` und HNSW-Index**
  In `internal/db/migrations/00009_pgvector_768.sql`:
  ```sql
  ALTER TABLE knowledge_base_articles ADD COLUMN IF NOT EXISTS embedding vector(768);
  ALTER TABLE knowledge_base_articles ADD COLUMN IF NOT EXISTS embedding_model TEXT;
  CREATE INDEX IF NOT EXISTS idx_kb_embedding_hnsw ON knowledge_base_articles USING hnsw (embedding vector_cosine_ops);
  ```
  `sqlc generate` ausführen.

- [ ] **Step 2: Embedding-Generierung im Gateway implementieren**
  - Methode `GenerateEmbedding(ctx, text) ([]float32, error)` via Ollama Embeddings API (`OLLAMA_EMBEDDING_MODEL`, Default `nomic-embed-text`) oder konfiguriertem Provider.
  - Dimensionen prüfen: Falls Modell ungleich 768 Dimensionen liefert, Serverstart mit klarer Fehlermeldung abbrechen.

- [ ] **Step 3: Semantische Suche im Handler aktivieren**
  - Cosinus-Distanz (`<=>`) in `ai.sql` für Ähnlichkeitssuche.

- [ ] **Step 4: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/ai/... ./internal/server/handlers/...
  git add internal/db internal/ai internal/server/handlers
  git commit -m "feat(ai): implement real 768-dim pgvector embeddings and semantic search (A9)"
  ```

---

# Phase 4: SP4 — Automation Engine

### Task 4.1: TriggerEvent verdrahten & Aktionen scharfschalten (U1–U6)

**Files:**
- Modify: `internal/core/automation/service.go`
- Modify: `internal/server/handlers/deals.go`
- Modify: `internal/server/handlers/connectors.go`
- Modify: `internal/core/email/service.go`
- Modify: `web/src/pages/AutomationsPage.tsx`
- Test: `internal/core/automation/service_test.go`

- [ ] **Step 1: Tests für TriggerEvent und Aktionen schreiben**
  - `TriggerEvent("NEW_LEAD")` startet passende Workflows.
  - `SET_TAG` fügt den Tag real in `contacts.tags` bzw. `deals.tags` ein.
  - `DRAFT_EMAIL` ruft AI-Gateway auf und speichert Draft in `email_messages`.
  - `NOTIFY_USER` nutzt tatsächliche `user_id` des Deal-/Kontakt-Owners.

- [ ] **Step 2: TriggerEvent an Event-Quellen aufrufen**
  - In `handlers/connectors.go`: Nach Lead-Erstellung `TriggerEvent("NEW_LEAD")`.
  - In `handlers/deals.go`: Bei Stage-Wechsel auf WON `TriggerEvent("DEAL_WON")`.
  - In `core/email`: Nach Inbound-Sync `TriggerEvent("INBOUND_EMAIL")`.

- [ ] **Step 3: Frontend AutomationsPage Buttons anbinden**
  - „Testlauf“ ruft `POST /api/v1/automations/{id}/test-run` auf.
  - „Schritt freigeben“ ruft `POST /api/v1/automations/runs/{id}/approve` auf.

- [ ] **Step 4: Tests & Build prüfen & Commit**
  ```bash
  go test -v ./internal/core/automation/... ./internal/server/handlers/...
  cd web && pnpm lint && pnpm build && cd ..
  git add internal/core/automation internal/server/handlers web/src/pages/AutomationsPage.tsx
  git commit -m "feat(automations): wire real event triggers, action handlers, and approval UI (U1-U6)"
  ```

---

# Phase 5: SP5 — Kalender, Telefonie & Geocoding

### Task 5.1: M365/Google Graph-Push streichen & RFC 5545 ICS härten (C1, C2)

**Files:**
- Modify: `web/src/pages/CalendarPage.tsx`
- Modify: `internal/server/handlers/appointments.go`
- Modify: `internal/core/appointment/service.go`
- Modify: `e2e/calendar-and-telephony.spec.ts`
- Test: `internal/server/handlers/appointments_test.go`

- [ ] **Step 1: Fake-M365/Google-Claims aus Frontend & E2E entfernen**
  - In `CalendarPage.tsx`: Entferne den Fake-Banner „M365 & Google OAuth verbunden“, Sync-Tags und unverbundene Push-Buttons.
  - In `e2e/calendar-and-telephony.spec.ts`: Entferne Assertions auf M365/Google-Push.

- [ ] **Step 2: `DownloadICS` ehrlich machen (C2, F-13)**
  - In `handlers/appointments.go`: Wenn Termin-ID nicht existiert → HTTP 404 (kein generischer Fake-Termin mehr!).
  - Valides RFC 5545 `.ics` mit `UID`, `DTSTART`, `DTEND`, `SUMMARY`, `LOCATION` generieren.

- [ ] **Step 3: Telefonie & Offline-PLZ-Geocoding (C3, C4)**
  - `CallModal.tsx`: Als „Anruf-Protokoll“ kennzeichnen mit echtem `tel:`-Link für Click-to-Call. Bei DB-Fehler echter HTTP 500.
  - `MapViewPage.tsx` & Geocoding: Offline-PLZ-Centroid-Tabelle für Deutschland einbinden. Kontakte ohne Koordinaten erhalten „Kein Standort“-Badge statt Frankfurt-Zufallswerte.

- [ ] **Step 4: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/server/handlers/... ./internal/core/appointment/...
  cd web && pnpm lint && pnpm build && cd ..
  git add web/src/ internal/server/handlers internal/core/appointment e2e/
  git commit -m "fix(calendar): remove fake M365/Google push, implement honest RFC 5545 ICS and PLZ geocoding (C1-C4, F-12, F-13)"
  ```

---

# Phase 6: SP6 — CRM Data Truth & UWG/BGB

### Task 6.1: Einwilligungen (UWG §7) und §355-BGB-Widerruf aktivieren (D1–D3, F-52)

**Files:**
- Modify: `internal/db/queries/contacts.sql`
- Modify: `internal/server/handlers/contacts.go`
- Modify: `internal/core/export/service.go`
- Modify: `web/src/pages/DealsPage.tsx`
- Modify: `internal/core/reports/service.go`
- Modify: `internal/core/contact/service.go`
- Test: `internal/server/handlers/contacts_test.go`
- Test: `internal/core/reports/service_test.go`

- [ ] **Step 1: Write-Path für Einwilligungen & Energiedaten verdrahten (D1)**
  - `CreateContact` und `UpdateContact` in `contacts.sql` um `consent_phone`, `consent_email`, `consent_updated_at`, `zaehlernummer`, etc. erweitern.
  - Handlers und CSV-Export anpassen, sodass echte Werte exportiert werden.

- [ ] **Step 2: §355 BGB Widerruf-Stage im Kanban & Reporting (D2)**
  - In `DealsPage.tsx`: Stage „Widerrufen“ ergänzen und Aktion „Widerruf eintragen“ anbieten.
  - In `reports/service.go`: Widerrufsquote anhand echter `widerrufen_at`-Einträge berechnen.

- [ ] **Step 3: Audit-Log PII-Sanitization (F-52)**
  - Keine Klartext-PII-Snapshots in Audit-Logs schreiben; Änderungen selektiv protokollieren.

- [ ] **Step 4: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/server/handlers/... ./internal/core/reports/...
  git add internal/db internal/server/handlers internal/core web/src/pages/DealsPage.tsx
  git commit -m "feat(crm): wire UWG consent fields, BGB revocation stage, and sanitize audit snapshots (D1-D3, F-52)"
  ```

---

# Phase 7: SP7 — Backup / Restore, Invites & Settings

### Task 7.1: Vollständiges Streaming-Backup & Restore (B1, B2)

**Files:**
- Modify: `internal/server/handlers/backup.go`
- Modify: `internal/core/backup/service.go`
- Test: `internal/server/handlers/backup_test.go`

- [ ] **Step 1: Export auf alle Tabellen als echter Stream erweitern**
  - Alle ~19 Tabellen zeilenweise über `json.Encoder` streamen (keine RAM-Akkumulation).

- [ ] **Step 2: Transaktionalen Restore-Endpoint implementieren**
  - `POST /api/v1/backup/restore`: Admin-only, prüft JSON, spielt Tabellen in Fremdschlüssel-Reihenfolge transaktional ein.
  - Restore-Drill führt echten Test-Import in Scratch-Schema durch.

---

### Task 7.2: Benutzer-Einladung & Settings-Fixes (B3, B4, B5, B6)

**Files:**
- Modify: `internal/server/handlers/users.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `internal/server/router.go`
- Test: `internal/server/handlers/users_test.go`

- [ ] **Step 1: Einladungs-Flow mit Token (B3)**
  - Einladungs-Token generieren und in DB speichern. Admin erhält **kopierbaren Einladungslink** direkt im UI angezeigt.
  - `GET /api/v1/auth/accept-invite?token=...`: Erlaubt Passwort-Setup und schaltet Account auf `ACTIVE`.

- [ ] **Step 2: 2FA-Toggle, Templates & Webhook-Test (B4, B5, B6)**
  - 2FA-Disable ruft echten `POST /api/v1/auth/totp/disable` auf.
  - Test-Webhook sendet korrekten `Authorization: Bearer`-Header und valides Payload.

- [ ] **Step 3: Tests & Build & Commit**
  ```bash
  go test -v ./internal/server/handlers/...
  cd web && pnpm lint && pnpm build && cd ..
  git add internal/server/handlers internal/core web/src/
  git commit -m "feat(backup): implement full streaming backup, restore, and token-based invite flow (B1-B6)"
  ```

---

# Phase 8: SP8 — Verbleibende Sicherheit

### Task 8.1: SP8a — Authentication, Sessions & RBAC

**Files:**
- Create: `internal/db/migrations/00010_revoked_tokens.sql`
- Modify: `internal/auth/service.go`
- Modify: `internal/auth/middleware.go`
- Modify: `internal/server/handlers/users.go`
- Modify: `internal/server/router.go`
- Test: `internal/auth/session_test.go`

- [ ] **Step 1: Migration für `revoked_tokens` und `must_change_password`**
  ```sql
  CREATE TABLE IF NOT EXISTS revoked_tokens (
      jti TEXT PRIMARY KEY,
      user_id UUID NOT NULL,
      expires_at TIMESTAMPTZ NOT NULL
  );
  ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN DEFAULT FALSE;
  ```
  `sqlc generate` ausführen.

- [ ] **Step 2: Session-Revocation & Refresh-Reuse-Detection implementieren (F-06, F-07)**
  - Bei Rollen-/Statusänderung alle Tokens des Nutzers widerrufen.
  - Wiederverwendung eines abgelaufenen Refresh-Tokens widerruft die gesamte Token-Familie.
  - Roh-Tokens aus JSON-Responses von Login/Refresh entfernen (nur HttpOnly Cookies, F-43).
  - Admin-Passwort beim Start nicht in Logs ausgeben; Flag `must_change_password` setzen (F-34).

- [ ] **Step 3: Rollen-Matrix durchsetzen (F-09, F-10, F-11, F-14)**
  - Fail-closed Ownership (`if claims == nil || ...`).
  - Routen absichern gemäß definierter Rollenmatrix für `ADMIN`, `VERTRIEB`, `BACKOFFICE`, `BENUTZER`.

- [ ] **Step 4: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/auth/... ./internal/server/...
  git add internal/db internal/auth internal/server
  git commit -m "fix(security): implement session revocation, refresh reuse detection, and enforce RBAC matrix (SP8a)"
  ```

---

### Task 8.2: SP8b — Network, Transport, CSRF, Uploads & Server Hardening

**Files:**
- Modify: `internal/auth/csrf.go`
- Modify: `internal/storage/local.go`
- Modify: `internal/ai/research.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/server/router.go`
- Modify: `docker-compose.yml`
- Test: `internal/storage/storage_test.go`

- [ ] **Step 1: CSRF HMAC-Session-Binding (F-16, F-18, F-19)**
  - CSRF-Token kryptographisch an die Session binden.

- [ ] **Step 2: Upload-MIME-Allowlist & CAS-Fix (F-24, F-25)**
  - Strikte Allowlist für PDFs/Bilder/CSV. MIME-Validierung **vor** dem Dedup-Stat-Check ausführen.

- [ ] **Step 3: SSRF & Updater absichern (F-28, F-29, F-30)**
  - DNS-Rebinding-Filter auf IPv4-mapped IPv6 erweitern.
  - Updater lädt nur Releases mit `SHA256SUMS`-Verifikation.

- [ ] **Step 4: Server Timeouts & Health-Endpoint (F-37, F-38, F-40, F-41, F-42)**
  - CSP/HSTS im Go-Layer setzen.
  - Standard `WriteTimeout: 30s` (SSE via `ResponseController.SetWriteDeadline`).
  - `/health` bleibt für lokalen Betrieb unauthentifiziert mit `demo_mode` erreichbar.
  - SSE `/events/stream` in protected API-Gruppe verschieben mit User-spezifischem Routing.
  - Docker Compose bindet Ports standardmäßig an `127.0.0.1`.

- [ ] **Step 5: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/storage/... ./internal/server/... ./cmd/server/...
  git add internal/storage internal/ai cmd/server internal/server docker-compose.yml
  git commit -m "fix(security): harden network transport, MIME uploads, SSRF, and server timeouts (SP8b)"
  ```

---

### Task 8.3: SP8c — Integrity, Cryptographic Audit Log & AI Guarding

**Files:**
- Create: `internal/db/migrations/00011_audit_hash_chain.sql`
- Modify: `internal/core/audit/service.go`
- Modify: `internal/ai/guard.go`
- Modify: `internal/ai/chat.go`
- Modify: `internal/ai/observability.go`
- Test: `internal/core/audit/audit_test.go`
- Test: `internal/ai/guard_test.go`

- [ ] **Step 1: Migration für Audit-Hashkette**
  ```sql
  ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS prev_hash TEXT;
  ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS hash TEXT;
  ```
  `sqlc generate` ausführen.

- [ ] **Step 2: Hashkette & Security-Event-Auditing (F-49, F-50, F-51)**
  - Jeder Eintrag berechnet SHA-256 über `prev_hash + timestamp + action + entity_id + user_id + changes`.
  - Audit-Logging für Login, Logout, TOTP, Rollenwechsel, Settings-Export und Backup.
  - Echte `ip_address` und `user_agent` protokollieren.

- [ ] **Step 3: Prompt Injection & Guard-Verfeinerung (F-35, F-46, F-48)**
  - Rollen-Marker filtern, System-Prompt isolieren.
  - AI Observability speichert `user_id`.
  - Regexes schärfen, um Über-Redaktion zu verhindern.

- [ ] **Step 4: Tests ausführen & Commit**
  ```bash
  go test -v ./internal/core/audit/... ./internal/ai/...
  git add internal/db internal/core/audit internal/ai
  git commit -m "fix(security): implement cryptographic audit log hash chain and prompt injection defense (SP8c)"
  ```

---

# Phase 9: SP9 — Docs, Tests, Config Truth & Demo Mode

### Task 9.1: Tote Endpoints bereinigen, Vitest & Real-DB E2E (P2, S1–S5)

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/simulation/SimulationWidget.tsx`
- Modify: `web/src/components/layout/AppLayout.tsx`
- Modify: `web/src/pages/LoginPage.tsx`
- Modify: `web/src/components/common/CommandPalette.tsx`
- Modify: `e2e/` (alle Specs)
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Delete: `docs/AUDIT_REPORT_2026-09-21.html`

- [ ] **Step 1: Vitest für Frontend-Unit-Tests aufsetzen**
  In `web/`:
  - `pnpm add -D vitest @testing-library/react jsdom`
  - Tests für Komponenten und AuthContext schreiben.

- [ ] **Step 2: Simulation nur in `DEMO_MODE` laden (S1–S5)**
  - `SimulationWidget` per `React.lazy` nur laden und mounten, wenn `isDemo === true`.
  - `LoginPage.tsx`: `isDemo` defaultet auf `false`; `demo123` wird in Produktion niemals vorausgefüllt.

- [ ] **Step 3: E2E-Tests gegen echte PostgreSQL-Instanz ausführen**
  - Specs so umschreiben, dass sie echte Persistenz in der DB validieren statt nur UI-Toasts.

- [ ] **Step 4: Dokumentation bereinigen & altes HTML-Audit entfernen**
  - README und CHANGELOG an den realen Zustand anpassen (keine Versprechungen über nicht existierende Features).
  - `docs/AUDIT_REPORT_2026-09-21.html` löschen und Demo-Credentials rotieren.

- [ ] **Step 5: Gesamte Quality Gate Prüfung (`make check`)**
  ```bash
  make check
  make test-e2e
  ```

- [ ] **Step 6: Commit**
  ```bash
  git add web/ e2e/ README.md CHANGELOG.md
  git rm docs/AUDIT_REPORT_2026-09-21.html
  git commit -m "chore: setup vitest, align docs with reality, and isolate demo simulation (SP9)"
  ```

---

## Final Verification & Hand-off

Nach Durchlauf aller 9 Phasen:
1. `make check` durchlaufen lassen (Fmt, Lint, Licensen, Tests).
2. Coverage-Gate prüfen: `scripts/coverage.sh --min 50.0 --diff 50.0`.
3. Round-5 Sweep durchführen: Verifizieren, dass null Fake-Success-Pfade mehr im Code existieren.
