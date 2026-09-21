# Changelog

Alle nennenswerten Änderungen an **OpenLocalCRM** werden in dieser Datei dokumentiert.
Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/) und folgt [Semantic Versioning](https://semver.org/).

---

## [1.0.5] - 2026-09-21

### Behoben & Gehärtet (Repo Review Round 2 Remediation)
- **Sicherheit & Auth:**
  - `localStorage` JWT-Bereinigung: Tokens werden nicht mehr im Web Storage gespeichert; Authentifizierung und CSRF-Handling erfolgen ausschließlich zustandslos über HttpOnly-Cookies (`credentials: 'include'`) und `/api/v1/me`.
  - `.dockerignore` strikt gehärtet gegen Secret-Leaks (`.env`, `.env.*`, `backups/`, `data/`, SQL-Dumps ausgeschlossen).
  - Dynamisches Cookie-Flag `Secure` mit Unterstützung für `X-Forwarded-Proto: https` und `FORCE_SECURE_COOKIES`.
  - Cookie-Gültigkeitsdauer auf `AccessTokenDuration` (30 Minuten) synchron zum Ed25519-Token verkürzt.
  - `RateLimitMiddleware` gegen IP-Spoofing abgesichert: Header wie `X-Forwarded-For` und `X-Real-IP` werden nur bei Verbindungen über vertrauenswürdige Loopback-/Private-Netzwerke akzeptiert.
  - Standard-Sicherheits-Header auf App-Ebene verankert (`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`).
  - Direkte Portexposition `8080:8080` in `docker-compose.demo.yml` entfernt; Routing erfolgt isoliert über Caddy.
- **KI-Gateway & Observability:**
  - Kontrollierter Failsafe-Modus bei unkonfigurierten KI-Providern (`anthropic`, `gemini`); strukturierte Transparenzlogs (`[AI_GATEWAY_NOTICE]`) bei Simulations-Fallbacks.
  - `ParseBill` liefert Transparenzflag `simulated: true` aus.
  - Strikte UUID-Validierung bei Wissensdatenbank-Löschungen (`DeleteKB`).
  - Fehlerprüfung bei automatischer Firmenerstellung in Chat-Absichten.
  - Reale KI-Firmenrecherche via `ResearchCompany` statt Dummy-Payload.
- **Automatisierungs-Engine:**
  - Reale Ausführung von Workflow-Aktionen: `CREATE_TASK` erstellt persistente Aufgaben, `NOTIFY_USER` generiert In-App Benachrichtigungen, `SET_TAG` erfasst Audit-Trail.
- **API, Paginierung & Backup-Integrität:**
  - Paginierungs-Unterstützung (`limit` und `offset`) für `/api/v1/appointments` und phasengefilterte Deals `/api/v1/deals?stage=...`.
  - Vollständige Fehlererkennung bei Backup-Restore-Drills (`/api/v1/backup/drill` meldet Tabellenfehler strikt mit HTTP 500).
  - Streaming-Export `/api/v1/backup/export` in 1.000er Batches ohne Speicherüberlauf oder stille Datenabschneidung.
  - Validierung von `deal_value` auf gültige Fließkommazahlen im Service und Webhook-Connector.
- **Frontend & Einstellungen:**
  - Interaktive QR-Code-Darstellung für 2FA-TOTP via `qrcode.react` (`QRCodeSVG`).
  - Echter REST-Payload-Versand für Webhook Lead-Intake Tests im Konnektor-Tab.
  - Konfigurierbarer API-Bearer-Token für externe Lead-Quellen.
  - Schema- und Typprüfung beim Einstellungs-Import (`handleImportSettings`).
  - Korrektur der Versionsanzeige auf Go 1.26 in den System-Informationen.

---

## [1.0.4] - 2026-09-21

### Behoben & Gehärtet (Security Audit & Repo Review)
- **Authentifizierung:** 2FA- und Passwort-Backdoors (`123456`, `demo123`, `oldpassword123`) restlos entfernt.
- **Timing-Schutz:** Dummy-Argon2id-Hash bei unbekannten Benutzern zur Abwehr von User-Enumeration.
- **Session-Sicherheit:** Umstellung auf HttpOnly-Cookies mit CSRF-Double-Submit-Token (`X-CSRF-Token`) und Single-Use Refresh-Token-Rotation mit 10s Grace-Period.
- **Rollenmodell:** Erweiterung des `user_role`-Enums um `VERTRIEB` und `BACKOFFICE` (Migration `00011_extend_user_roles.sql`) mit Frontend-Unterstützung.
- **Rate-Limiting:** Bounded Sliding-Window Rate-Limiter (10.000er Memory-Cap mit automatischem Cleanup) für alle Auth- und KI-Endpunkte.
- **Zugriffskontrolle & IDOR:** Strikte Ownership- und Rollenprüfungen für Notizen, Aufgaben, Termine und Benachrichtigungen.
- **KI-Sicherheit & EU AI Act:** Steuer-ID/VAT-Maskierung (`[REDACTED_TAXID]`), Prompt-Injection-Kapselung (`<untrusted_user_context>`) und persistentes Audit-Ledger in `ai_audit_logs`.
- **Automatisierung:** Vollständige HITL-Ausführung (`SET_TAG`, `CREATE_TASK`, `DRAFT_EMAIL`, `ApproveStep`) und reale River-Queue-Worker.
- **Speicher & Indizes:** Content-Addressable Storage (CAS SHA-256 Deduplizierung), MIME-Type-Prüfung und Foreign-Key-Indizes (`00010_foreign_key_indexes.sql`).
- **Repo-Hygiene:** Kompilierte Binaries aus dem Git-Tracking entfernt, Standard-OSS-Richtlinien ergänzt.

---

## [1.0.3] - 2026-09-21

### Behoben
- KI-Copilot Modell-Konfiguration (Mistral/Gemma Fallbacks) und dynamische Modell-Badges in der UI.

---

## [1.0.2] - 2026-09-21

### Hinzugefügt
- Settings-Export & Import bei Container-Rebuilds ohne zwingendes DB-Backup.

---

## [1.0.1] - 2026-09-21

### Behoben
- Caddyfile Edge-Proxy Konfiguration für lokales und Cloud-Hosting.

---

## [1.0.0] - 2026-09-21

### Erstveröffentlichung
- Single-Tenant CRM Kernarchitektur (Kontakte, Firmen, Deals, Termine, E-Mails).
- Integriertes React 18 SPA mit Tailwind CSS und Cmd+K Schnellsuche.
- Lokale Gemma 12B KI-Integration via Ollama mit DSGVO-Datenschutzfiltern.
- PostgreSQL 16 mit pgvector-Vektorsuche und automatischer Schema-Migration.
