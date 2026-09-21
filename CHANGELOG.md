# Changelog

Alle nennenswerten Änderungen an **OpenLocalCRM** werden in dieser Datei dokumentiert.
Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/) und folgt [Semantic Versioning](https://semver.org/).

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
