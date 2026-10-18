# 41 Audit-Befunde Behebung Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Vollständige und lückenlose Behebung aller 41 Befunde (5 Kritisch, 9 Hoch, 18 Mittel, 9 Niedrig) aus dem Audit-Bericht vom 21.09.2026 für `openlocalcrm`, inklusive Integration in den Setup-Client (`internal/launcher`).

**Architecture:** Beseitigung aller Backdoors; Einführung einer sicheren HttpOnly-Cookie-Architektur mit CSRF-Double-Submit-Protection und Single-Use Refresh-Rotation; persistentes EU-AI-Act Audit-Ledger (`ai_audit_logs`) mit Steuer-ID-Maskierung und Prompt-Injection-Kapselung; funktionale HITL-Automation-Engine mit realen River-Queue-Workern; RBAC- und IDOR-Schutz; CAS-Dateispeicher-Deduplizierung; saubere Abstimmung mit dem Setup-Client für Modellwechsel und Container-Rebuilds.

**Tech Stack:** Go 1.22+, Chi v5 Router, PostgreSQL 16 / pgx v5, sqlc, River Queue, Argon2id, RFC 6238 TOTP, Ed25519 JWT, React 18 / TypeScript, Vite, Tailwind CSS.

## Global Constraints
- Alle Tests müssen mit `go test ./...` fehlerfrei durchlaufen.
- Frontend muss mit `npm run build` in `web/` ohne Typfehler kompilieren.
- Keine verbleibenden Backdoors oder Test-Bypässe im Produktionscode.
- Revisionssicherheit nach Art. 50/52 EU AI Act muss real in PostgreSQL persistiert werden (mit RAM-Fallback für Demo-Modus).
- `docs/superpowers/` bleibt in `.gitignore` und wird nicht in Git getrackt.
- Setup-Client (`internal/launcher`) muss bei Klick auf "Container aktualisieren (Rebuild)" stabil und fehlerfrei funktionieren.

---

### Task 1: Auth-Backdoors entfernen & Timing-Schutz (Befunde 1, 2, 33, 35)

**Files:**
- Modify: `internal/auth/totp.go:20-25`
- Modify: `internal/auth/service.go:60-75, 233-257`
- Modify: `internal/auth/jwt.go:50-70`
- Test: `internal/auth/service_test.go`
- Test: `internal/auth/totp_test.go`

**Interfaces:**
- Consumes: `ValidateTOTPCode(passcode, secret string) bool`, `ChangePassword(ctx, userID, oldPassword, newPassword) error`
- Produces: Sichere TOTP- und Passwort-Validierung ohne Sonderfallprüfung

- [ ] **Step 1: Test für Backdoor-Beseitigung und Dummy-Timing schreiben**
  Erstelle Tests in `internal/auth/totp_test.go` und `internal/auth/service_test.go`:
  - `TestValidateTOTPCode_NoBypass`: Code `"123456"` schlägt fehl, wenn das Secret nicht dazu passt.
  - `TestChangePassword_NoBypasses`: `"oldpassword123"` und `"demo123"` werden abgewiesen, wenn sie nicht mit dem echten Hash übereinstimmen.
  - `TestValidateAccessToken_IssuerAudience`: Prüft, dass Token mit falschem `iss` abgewiesen werden.

- [ ] **Step 2: Tests ausführen und Fehlschlag verifizieren**
  Befehl: `go test -v -run "TestValidateTOTPCode_NoBypass|TestChangePassword_NoBypasses" ./internal/auth`
  Erwartung: FAIL (da `"123456"` und `"demo123"` aktuell noch als gültig durchgehen).

- [ ] **Step 3: Backdoors entfernen & Timing-Schutz implementieren**
  1. In `internal/auth/totp.go`:
     ```go
     func ValidateTOTPCode(passcode, secret string) bool {
         if len(passcode) != 6 || secret == "" {
             return false
         }
         return totp.Validate(passcode, secret)
     }
     ```
  2. In `internal/auth/service.go`:
     - Entfernen von `&& oldPassword != "oldpassword123" && oldPassword != "demo123"`.
     - In `Login`: Bei `err == ErrUserNotFound` einen Dummy-Hash `CheckPassword("$argon2id$v=19$m=65536,t=1,p=4$dummy$dummy", password)` ausführen.
  3. In `internal/auth/jwt.go`:
     - Im Token-Parser `jwt.WithIssuer("openlocalcrm")` und `jwt.WithAudience("openlocalcrm-api")` erzwingen.

- [ ] **Step 4: Tests erneut ausführen und Pass verifizieren**
  Befehl: `go test -v ./internal/auth`
  Erwartung: PASS.

---

### Task 2: Session-Sicherheit: HttpOnly-Cookies, CSRF-Schutz & Token-Revocation (Befunde 6, 7, 14, 25, 26)

**Files:**
- Create: `internal/server/csrf.go`
- Create: `internal/server/csrf_test.go`
- Modify: `internal/auth/service.go:134-157`
- Modify: `internal/server/handlers/auth.go:90-185`
- Modify: `internal/server/router.go:104-130`
- Modify: `web/src/api/client.ts:20-60`
- Modify: `web/src/hooks/useSSE.ts:15-35`

**Interfaces:**
- Consumes: `CSRFMiddleware(next http.Handler) http.Handler`
- Produces: `X-CSRF-Token`-Header-Prüfung, Single-Use Refresh-Rotation mit 10s Grace-Period, serverseitige Revocation

- [ ] **Step 1: Test für CSRF-Schutz und Refresh-Token-Rotation schreiben**
  - Test für `CSRFMiddleware`: Blockiert mutierende Anfragen ohne passenden `X-CSRF-Token`, erlaubt GET/HEAD und Requests mit `Authorization: Bearer`.
  - Test für `RefreshToken_SingleUseAndGrace`: Nach 1x Einlösen neuer Token; altes Token innerhalb von 10s liefert gecachten Token; nach >10s Replay-Sperre.

- [ ] **Step 2: CSRF-Middleware und Refresh-Rotation implementieren**
  1. `internal/server/csrf.go`:
     - Generiert kryptografisches 32-Byte CSRF-Token, setzt `csrf_token` Cookie (readable).
     - Prüft bei `POST`, `PUT`, `DELETE`, `PATCH` den Header `X-CSRF-Token` gegen das Cookie.
     - Ignoriert Prüfung, wenn `Authorization: Bearer <token>` gesetzt ist.
  2. `internal/auth/service.go`:
     - In `RefreshToken`: `s.querier.DeleteRefreshToken(ctx, tokenHash)`.
     - In-Memory Grace-Cache (`sync.Map`) speichert gelöschte Hashes für 10 Sekunden.
     - Bei Replay nach Ablauf: `s.querier.DeleteUserRefreshTokens(ctx, user.ID)`.
  3. `internal/server/handlers/auth.go`:
     - `Logout`: Extrahiert `refresh_token`-Cookie, ruft `s.querier.DeleteRefreshToken`, trägt `jti` in Access-Token-Blacklist ein, löscht Cookies.
  4. `internal/server/router.go`:
     - Entferne `?token=` aus `/events/stream`. Authentifiziere rein über Cookie oder Header.
  5. `web/src/api/client.ts` & `web/src/hooks/useSSE.ts`:
     - Entferne `localStorage.getItem('auth_token')`.
     - `apiFetch`: Lese `csrf_token`-Cookie aus und setze `X-CSRF-Token`-Header bei Mutationen.
     - `useSSE`: Verbinde auf `/events/stream` ohne Query-Parameter (Browser sendet Cookie automatisch).

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/server ./internal/auth`
  Erwartung: PASS.

---

### Task 3: RealIP Rate-Limiter & Schutz sensibler Endpunkte (Befunde 8, 34)

**Files:**
- Modify: `internal/auth/limiter.go:1-80`
- Modify: `internal/server/router.go:300-370`
- Test: `internal/auth/limiter_test.go`

**Interfaces:**
- Consumes: `limiter.Allow(key string) bool`, `limiter.RecordFailure(key string)`
- Produces: Caddy-Reverse-Proxy-fähiges Rate-Limiting mit Memory-Cap

- [ ] **Step 1: Test für Rate-Limiter Kapazität und Cleanup schreiben**
  - `TestRateLimiter_CapacityCap`: Bei >10.000 Schlüsseln wird ältester Eintrag eviziert.
  - `TestRateLimiter_CleanupExpired`: Abgelaufene Einträge werden freigegeben.

- [ ] **Step 2: Rate-Limiter härten**
  1. In `internal/auth/limiter.go`:
     - Maximal 10.000 aktive Tracker in der Map.
     - `time.NewTicker(10 * time.Minute)` räumt im Hintergrund abgelaufene Fenster ab.
  2. In `internal/server/router.go`:
     - Chi `middleware.RealIP` vor den Handlern aktivieren, sodass `r.RemoteAddr` die echte Client-IP aus `X-Forwarded-For` enthält.
     - Rate-Limiting auf `/api/v1/auth/refresh`, `/api/v1/auth/totp/*`, `/api/v1/auth/change-password` und `/api/v1/ai/*` schalten.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/auth`
  Erwartung: PASS.

---

### Task 4: RBAC & IDOR-Schutz auf Notizen, Todos, Terminen & Demo-Ingest (Befunde 11, 12, 27, 41)

**Files:**
- Modify: `internal/server/handlers/appointments.go:100-140`
- Modify: `internal/server/handlers/notes.go:40-90`
- Modify: `internal/server/handlers/todos.go:50-100`
- Modify: `internal/server/handlers/notifications.go:30-45`
- Modify: `internal/server/handlers/emails.go:60-80`
- Modify: `internal/server/router.go:350-380`
- Test: `internal/server/handlers/appointments_test.go`
- Test: `internal/server/handlers/emails_test.go`

**Interfaces:**
- Consumes: JWT-Claims (`user_id`, `role`)
- Produces: Strikte Autorisierung für `ADMIN`, `VERTRIEB`, `BACKOFFICE`

- [ ] **Step 1: Tests für IDOR und Demo-Ingest-Sperre schreiben**
  - `TestAppointments_IDOR`: Vertriebler kann fremden Termin nicht löschen (403); Admin kann löschen (200).
  - `TestDemoIngest_ProductionBlocked`: Wenn `DEMO_MODE=false`, gibt `/api/v1/emails/demo-ingest` 403 Forbidden zurück.
  - `TestSalesReport_Restricted`: Nur `ADMIN` und `BACKOFFICE` dürfen den Report abrufen.

- [ ] **Step 2: Handler-Prüfungen implementieren**
  1. `appointments.go`:
     - `GetByID`, `Update`, `Delete` prüfen Ownership: Admin darf alles; Vertriebler nur eigene; Backoffice darf nicht löschen (403). Fehler von `svc.Delete` werden zurückgegeben.
  2. `notes.go` & `todos.go`:
     - IDOR-Prüfung auf `Update` und `Delete`.
  3. `notifications.go`:
     - `MarkRead` prüft, ob die Notiz dem eingeloggten `actorID` zugeordnet ist.
  4. `emails.go`:
     - In `DemoIngest`: `if !h.demoMode { http.Error(w, "forbidden in production", http.StatusForbidden); return }`.
  5. `router.go`:
     - `/api/v1/reports/sales` mit `RequireRoles("ADMIN", "BACKOFFICE")` schützen.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/server/handlers`
  Erwartung: PASS.

---

### Task 5: KI-Guard, Steuer-ID-Maskierung & Prompt-Injection-Schutz (Befunde 4, 9, 10, 29, 38)

**Files:**
- Modify: `internal/ai/guard.go:10-50`
- Modify: `internal/ai/gateway.go:80-120`
- Modify: `internal/ai/chat.go:130-160`
- Modify: `internal/ai/research.go:215-230`
- Test: `internal/ai/guard_test.go`
- Test: `internal/ai/research_test.go`

**Interfaces:**
- Consumes: `guard.Sanitize(text string) (clean string, count int, triggered bool)`
- Produces: Vollständige PII-Redaktion inkl. Steuer-ID, Kapselung von Kontext in XML-Tags

- [ ] **Step 1: Tests für Steuer-ID-Maskierung und Prompt-Injection schreiben**
  - `TestGuard_TaxIDRedaction`: 11-stellige Nummer wird zu `[REDACTED_TAXID]` maskiert.
  - `TestGuard_RedactionCounter`: Liefert korrekte Anzahl maskierter Entitäten zurück.
  - `TestResearch_RuneSafeTruncate`: Multi-Byte-UTF-8 (z. B. "Photovoltaikanlagen-Förderung ÄÖÜ") wird ohne Panic oder fehlerhafte Runen trunkiert.

- [ ] **Step 2: KI-Guard und Gateway härten**
  1. In `internal/ai/guard.go`:
     - `taxIDRegex.ReplaceAllString(sanitized, "[REDACTED_TAXID]")` in `SanitizeInput` einbinden.
     - Methode `Sanitize(text string) (cleanText string, count int, triggered bool)` bereitstellen.
     - `ValidateOutput` erweitert um Leak-Erkennung (z. B. Echo von "You are an AI assistant for OpenLocalCRM") und Entfernung gefährlicher HTML-Scripts (`<script>`).
  2. In `internal/ai/gateway.go`:
     - `systemInstruction` in `Generate` ebenfalls durch `SanitizeInput` schicken.
  3. In `internal/ai/chat.go`:
     - Client-Kontext nicht mehr in `systemInstruction` hängen, sondern in `<untrusted_user_context>` im Prompt einbetten.
  4. In `internal/ai/research.go`:
     - UTF-8 `[]rune`-Slicing für HTML-Scraping statt roher Byte-Slices.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/ai`
  Erwartung: PASS.

---

### Task 6: Revisionssicheres EU-AI-Act Audit-Ledger & Fallback-Transparenz (Befunde 3, 13, 30, 37, 39, 40)

**Files:**
- Modify: `internal/ai/observability.go:1-95`
- Modify: `internal/ai/triage.go:80-120`
- Modify: `internal/ai/chat.go:160-220`
- Modify: `internal/ai/gateway.go:95-115`
- Modify: `web/src/simulation/SimulationWidget.tsx:110-140`
- Test: `internal/ai/observability_test.go`
- Test: `internal/ai/triage_test.go`

**Interfaces:**
- Consumes: `obsSvc.Record(ctx, AIAuditLog)`
- Produces: Persistente Speicherung in `ai_audit_logs`, `simulated: true`-Flag

- [ ] **Step 1: Tests für Observability-Persistenz und Negative Limit schreiben**
  - `TestObservability_NegativeLimit`: `GetRecentLogs(-5)` panict nicht, sondern liefert Standard-Array.
  - `TestChat_SimulatedFlag`: Wenn LLM offline ist, setzt die Antwort `simulated: true`.
  - `TestTriage_AuditLogged`: Aufruf von `TriageEmail` erzeugt Eintrag mit `InteractionType = "TRIAGE"`.

- [ ] **Step 2: Observability und Triage erweitern**
  1. In `internal/ai/observability.go`:
     - `Record` schreibt via `db.DBTX` in `ai_audit_logs`. Im Demo-Modus Fallback auf das In-Memory-Array.
     - `GetRecentLogs`: Prüfe `if limit <= 0 { limit = 50 }`.
  2. In `internal/ai/triage.go`:
     - Binde `obsSvc` ein und rufe `Record()` nach jeder Triage auf.
  3. In `internal/ai/chat.go`:
     - Ergänze Feld `Simulated bool` in `ChatResponse`. Setze `true`, wenn `generateSmartFallback` genutzt wurde.
     - DB-Fehler als echten Fehler zurückgeben, nicht als „0 Deals".
  4. In `internal/ai/gateway.go`:
     - Entferne stilles Überschreiben des OpenAI-Modellnamens mit `gpt-4o`.
  5. In `web/src/simulation/SimulationWidget.tsx`:
     - Ersetze den direkten Browser-Fetch zu `localhost:11434` durch `apiFetch('/api/v1/ai/triage')`.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/ai`
  Erwartung: PASS.

---

### Task 7: HITL-Automation-Engine & River-Queue-Worker (Befunde 5, 16, 17)

**Files:**
- Modify: `internal/core/automation/service.go:50-216`
- Modify: `internal/queue/jobs.go:1-50`
- Modify: `internal/server/handlers/automations.go:40-80`
- Test: `internal/core/automation/service_test.go`
- Test: `internal/queue/jobs_test.go`

**Interfaces:**
- Consumes: `TriggerEvent(ctx, triggerType, targetID, targetName)`, `ApproveStep(ctx, runID)`
- Produces: Reale Workflow-Ausführung (Todos, Tags, E-Mails) und River-Queue-Jobs

- [ ] **Step 1: Tests für Workflow-Execution und Worker schreiben**
  - `TestAutomation_TriggerAndApprove`: Neuer Lead triggert Workflow; `ApproveStep` erzeugt Todo und setzt Status auf `COMPLETED`.
  - `TestAutomation_CreateWorkflowError`: DB-Fehler wird nicht maskiert, sondern als Fehler zurückgegeben.
  - `TestRiverQueue_TriageJob`: `EmailTriageWorker` führt Triage aus und speichert Ergebnis.

- [ ] **Step 2: Dispatch-Engine und Worker ausprogrammieren**
  1. In `internal/core/automation/service.go`:
     - `TriggerEvent`: Findet aktive Workflows, erstellt `WorkflowRun`, führt automatische Schritte aus oder setzt `WAITING_APPROVAL`.
     - `ApproveStep`: Führt `SET_TAG`, `CREATE_TASK`, `DRAFT_EMAIL` real aus und schaltet den Run weiter.
     - `CreateWorkflow`: Reiche `err` transparent an Aufrufer durch.
  2. In `internal/queue/jobs.go`:
     - `EmailTriageWorker`: Liest E-Mail, ruft `ai.TriageEmail` auf, speichert Ergebnis in DB, emittiert SSE-Event.
     - `EmailSyncWorker`: Ruft IMAP ab und speichert E-Mails.
     - Im Demo-Modus: Synchrone In-Memory-Verarbeitung ohne River-Client.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/core/automation ./internal/queue`
  Erwartung: PASS.

---

### Task 8: Domain-Services: CustomFields-Merge, Validierung & Concurrency (Befunde 15, 18, 19, 20, 36)

**Files:**
- Modify: `internal/core/deal/service.go:170-210`
- Modify: `internal/core/appointment/service.go:40-100`
- Modify: `internal/core/contact/service.go:40-80`
- Test: `internal/core/deal/service_test.go`
- Test: `internal/core/appointment/service_test.go`

**Interfaces:**
- Consumes: `UpdateDealInput` mit `CustomFields` und `ExpectedUpdatedAt`
- Produces: Saubere Datenvalidierung, Optimistic Concurrency Control, Audit-Log-Fehlerbehandlung

- [ ] **Step 1: Tests für Deal-CustomFields und Concurrency schreiben**
  - `TestDeal_UpdatePreservesCustomFields`: Update überschreibt bestehende `CustomFields` nicht mit `"{}"`.
  - `TestDeal_OptimisticLocking`: Veraltetes `updated_at` liefert `ErrOptimisticLockConflict` (409).
  - `TestDeal_Validation`: Negativer Betrag oder Wahrscheinlichkeit >100 liefert Validierungsfehler.
  - `TestAppointment_Validation`: `EndTime` vor `StartTime` liefert Validierungsfehler.

- [ ] **Step 2: Core-Services anpassen**
  1. `deal/service.go`:
     - `UpdateDeal`: Lade bestehenden Deal und merge `CustomFields`.
     - Prüfe `updated_at` gegen `input.ExpectedUpdatedAt`.
     - Validiere `Value >= 0` und `0 <= Probability <= 100`.
     - Logge Audit-Fehler mit `log.Printf("[AUDIT_ERROR] ...")`.
  2. `appointment/service.go`:
     - Validiere `EndTime.After(StartTime)`.
  3. `contact/service.go`:
     - Validiere E-Mail-Format.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/core/deal ./internal/core/appointment ./internal/core/contact`
  Erwartung: PASS.

---

### Task 9: Fremdschlüssel-Indizes, Migrationsbereinigung & Storage CAS (Befunde 21, 22, 28, 31, 32)

**Files:**
- Create: `internal/db/migrations/00009_foreign_key_indexes.sql`
- Modify: `internal/storage/local.go:40-85`
- Modify: `internal/core/email/service.go:45-60`
- Delete redundant: `migrations/` Root-Ordner (konsolidiert auf `internal/db/migrations/`)
- Test: `internal/storage/local_test.go`

**Interfaces:**
- Consumes: `LocalStorage.SaveFile`
- Produces: Echte SHA-256 CAS-Deduplizierung, MIME-Type-Schutz, AES-GCM-Mail-Passwort-Verschlüsselung

- [ ] **Step 1: Tests für Storage CAS und MIME-Type schreiben**
  - `TestLocalStorage_Deduplication`: Zwei Uploads desselben Inhalts mit unterschiedlichen Dateinamen speichern nur 1 physische Datei.
  - `TestLocalStorage_MIMEValidation`: HTML/SVG mit Script-Tags wird abgewiesen.

- [ ] **Step 2: Indizes anlegen & Storage härten**
  1. `internal/db/migrations/00009_foreign_key_indexes.sql`:
     ```sql
     CREATE INDEX IF NOT EXISTS idx_contacts_company_id ON contacts(company_id);
     CREATE INDEX IF NOT EXISTS idx_deals_company_id ON deals(company_id);
     CREATE INDEX IF NOT EXISTS idx_deals_contact_id ON deals(contact_id);
     CREATE INDEX IF NOT EXISTS idx_todos_contact_id ON todos(contact_id);
     CREATE INDEX IF NOT EXISTS idx_todos_deal_id ON todos(deal_id);
     ```
  2. Entferne das redundante Verzeichnis `migrations/` an der Root-Ebene (oder richte einen Symlink auf `internal/db/migrations` ein), damit nur ein einziger Pfad existiert.
  3. `internal/storage/local.go`:
     - CAS-Prüfung: Falls Datei mit Hash existiert, Pfad wiederverwenden.
     - MIME-Type-Check: Erste 512 Bytes per `http.DetectContentType` prüfen.
  4. `internal/core/email/service.go`:
     - AES-256-GCM Verschlüsselung für `PasswordEncrypted`.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/storage`
  Erwartung: PASS.

---

### Task 10: Setup-Client & Launcher-Integration mit Rebuild/Update Flow (Befunde 23, 24)

**Files:**
- Modify: `internal/launcher/ui/app.js:385-405`
- Modify: `internal/launcher/ui/index.html:190-200`
- Modify: `internal/launcher/settings.go:90-110`
- Modify: `docker-compose.yml:1-50`
- Modify: `Caddyfile:1-30`
- Test: `internal/launcher/server_settings_test.go`
- Test: `internal/launcher/ai_probe_test.go`

**Interfaces:**
- Consumes: Setup-Client UI Port 8085, Docker Engine API
- Produces: Nahtlose Synchronisation von Modellnamen, `.env`-Parametern und Rebuilds

- [ ] **Step 1: Tests für Setup-Client Settings und Rebuild schreiben**
  - `TestLauncher_AIProbeDefaultModel`: Probe nutzt `gemma4:12b` für Ollama und `gpt-4o-mini` für OpenAI.
  - `TestLauncher_EnvPasswordRequired`: Generiert `.env` mit sicherem Zufallspasswort für `DB_PASSWORD`.

- [ ] **Step 2: Launcher & Docker-Konfiguration anpassen**
  1. `docker-compose.yml`:
     - `POSTGRES_PASSWORD: ${DB_PASSWORD:?DB_PASSWORD must be set in .env}`.
  2. `Caddyfile`:
     - Klare Struktur für lokale Entwicklung und Produktions-Domain mit automatischem TLS/HSTS.
  3. `internal/launcher/ui/app.js` & `index.html`:
     - Standard-Modell auf `gemma4:12b` setzen (Ollama) bzw. `gpt-4o-mini` (OpenAI).
  4. `internal/launcher/settings.go`:
     - Automatische Generierung von `DB_PASSWORD` in `.env`, falls leer.
     - Absicherung, dass `Container aktualisieren (Rebuild)` die Container ohne Datenverlust oder Auth-Bruch neu startet.

- [ ] **Step 3: Tests ausführen**
  Befehl: `go test -v ./internal/launcher`
  Erwartung: PASS.

---

### Task 11: End-to-End Verifikation & Frontend Production Build

**Files:**
- Run: `go test ./...`
- Run: `npm run build` in `web/`
- Test: `internal/server/demo_e2e_test.go`

- [ ] **Step 1: Backend Gesamttestlauf ausführen**
  Befehl: `go test -v ./internal/...`
  Erwartung: Alle Tests laufen zu 100% grün durch.

- [ ] **Step 2: Frontend Production Build ausführen**
  Befehl: `cd web && npm run build`
  Erwartung: Erfolgreicher Build ohne Typ- oder Bundler-Fehler.

- [ ] **Step 3: Dokumentationsabgleich**
  Aktualisiere `README.md` und `docs/API_REFERENCE.md` bezüglich der CSRF-Token-Vorgabe und der gehärteten Endpunkte.
