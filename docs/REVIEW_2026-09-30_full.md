# Full Source Review — 2026-09-30 (HEAD c6bac3e)

Teilberichte: `docs/review/2026-09-30/00…08-*.md` (dort alle Einzel-Findings mit Beleg `datei:zeile`).
Vorgehen: statische Analyse + Scanner (`go vet`, `gofmt`, `go test -race`, `govulncheck`, `gosec`, `pnpm lint/typecheck/audit`). Kein Live-Test gegen echte Postgres/Docker. Verifiziert vom Controller im Code: F-5-01, Rechte-Konstanten ungenutzt, Diff-Coverage ohne `-base`, `UpdateDeal` überschreibt alle Spalten, Key-Default `data/secrets.key` nicht auf Volume.
`golangci-lint` v1.64.8 läuft mit Go 1.27.1 nicht (typecheck-Fehler): **Lint-Gate unverifiziert.**

## Gesamtbewertung

| Dimension | Note | Begründung |
| --- | --- | --- |
| Features | C+ | Breite vorhanden, aber stille In-Memory-Fallbacks, tote Rechte, Einwilligung/Löschung nicht durchgesetzt. |
| Testabdeckung | B- | 59,6 % gesamt, Kernlogik gut. Aber: sqlc/Postgres-Pfade ungetestet in CI, Diff-Gate wirkungslos, Web 0 Tests. |
| Sicherheit | D+ | Solide Basis (Argon2id, Body-Limits, 0 govulncheck-Treffer), aber mehrere S1: Datenleck über SSE/Mail, Launcher-API offen, Auth-Flow defekt. |
| Datenschicht | D | Frische Installation vermutlich unmöglich (00010), Backup nicht restorebar, Audit-Kette wirkungslos. |
| Code-Qualität Backend | B- | Saubere Struktur, Timeouts/Shutdown ok; Fehler werden verschluckt, Ressourcen unbegrenzt (IMAP, Listen). |
| Frontend | B- | Lint/Typecheck/Audit sauber; CSRF-Retry-Bug, Drittanbieter-Assets, Cache bei Logout. |
| Betrieb/Lieferkette | C- | CI ohne Lint/Coverage/Postgres, unsignierter Updater, widersprüchliche CSP, Schlüssel nicht persistiert. |
| Compliance | C- | KI-Doku nicht durch Code gedeckt; keine Einwilligungsprüfung, kein Löschkonzept. |

## Release-Blocker (S1)

| ID (konsolidiert) | Finding | Belege (Teilberichte) |
| --- | --- | --- |
| R-01 | Migration `00010` indiziert `notes(contact_id)`, Spalte existiert nie → frische Postgres-Installation bricht ab. Controller bestätigt statisch; Live-Test offen. | F-5-01, F-8-01 |
| R-02 | SSE `Broadcast` sendet Notizen, private Termine, Mail-Betreff/Absender an **alle** eingeloggten Nutzer, umgeht Ownership. | F-1-02, F-3-02, F-6-01 |
| R-03 | Mail-Nachrichten nicht nach Postfach/Rolle gefiltert: jeder Nutzer liest/taggt alle Postfächer. | F-3-01 |
| R-04 | `UpdateDeal`/Todo-/Kalkulations-Update setzt Kontakt, Firma, Owner auf NULL; neue Deals/Todos ohne Owner. Controller: SQL überschreibt alle Spalten. | F-3-03 |
| R-05 | Master-Key (TOTP + Mail-Passwörter) liegt nicht auf Volume, Worker erzeugt eigenen → nach Rebuild 2FA-Aussperrung, Zugangsdaten verloren. | F-3-04 |
| R-06 | Backup seit SP8c nicht restorebar (Export enthält `prev_hash`/`hash`, Allowlist lehnt ab). | F-4-01, F-5-02 |
| R-07 | Launcher-HTTP-API (127.0.0.1:9099) ohne Auth/Origin-Check: Cross-Site-POST kann Passwort-Reset, Restore, Update, Container-Kontrolle auslösen. Nicht praktisch getestet. | F-8-02 |

## Hoch (S2), gruppiert

**Auth/Rechte**
- R-08 `/auth/refresh` und `/auth/logout` liefern laut Agent-Test 403 durch CSRF-Bindung (Logout widerruft nichts, Refresh funktioniert nie); Frontend-Agent sieht verwandten Fehler (alter `X-CSRF-Token` im Retry). Ein Root-Cause, Live-Verifikation nötig. F-3-05, F-7-01
- R-09 Sechs Permissions nie durchgesetzt (`PermContactsWrite/Delete`, `PermNotesWrite`, `PermEmailsSend/Team`, `PermOwnOnly`); BENUTZER darf Deals schreiben; RBAC-Routentest deckt ~10 von ~100 Routen. F-1-07, F-2-05, F-3-07
- R-10 Rollen-/Status-/Passwortwechsel erhöhen `token_version` nicht (SP8a §3.4 nicht umgesetzt). F-3-06
- R-11 Mail-Account auf fremden Host umstellen → Verbindungstest sendet gespeichertes Passwort dorthin. F-3-08
- R-12 Admin-Passwort aus `.env.example` wird unverändert übernommen. F-3-09

**Integrität/Datenverlust**
- R-13 Audit-Hash-Kette wirkungslos: Race beim Anhängen, JSONB-Normalisierung, Verify überspringt NULL-`prev_hash`, Restore verliert Hashes, Audit außerhalb der Geschäfts-Transaktion. F-4-02, F-5-02..04, F-5-06
- R-14 Notizen/Termine fallen bei DB-Fehler still auf In-Memory zurück und melden 201 → Datenverlust. F-1-01, F-4-04
- R-15 IMAP-Sync lädt alles in RAM, ohne Timeout/Context; Fehler werden verschluckt, `LastUid` rückt trotzdem vor → Mailverlust; `message_id` global statt je Account eindeutig. F-4-05, F-6-03, F-6-04
- R-16 Inaktivitäts-Scan triggert stündlich dieselben 100 ältesten Kontakte und legt Todos doppelt an. F-6-02

**Compliance**
- R-17 Einwilligung (Telefon/E-Mail) gespeichert, aber vor Anruf/Mail/Automation nie geprüft (UWG); kein Art.-15-Export, kein Löschkonzept/Retention; Notizen ohne FK zu Kontakten. F-1-03, F-1-04
- R-18 PII-Guard erkennt nur IBAN/Kreditkarte/Steuer-ID/Secrets, nicht Name/Adresse/Telefon/E-Mail; PII-Zähler hart `false`; `WEBHOOK`-Aktion läuft ohne Freigabe; `ai_audit_logs` änderbar; Doku-Zusagen nicht gedeckt. F-4-03, F-8-14

**Lieferkette/CI**
- R-19 CI ohne golangci-lint, Coverage-Gate, Postgres-Integration, SHA-Pins; Images ohne grüne CI publiziert, ohne Signatur/SBOM. F-2-02, F-2-03, F-8-04..07
- R-20 Pre-Push-Diff-Gate wirkungslos (`diff-coverage.go` ohne `-base`, leerer Diff → Exit 0). Controller bestätigt im Code. F-2-01
- R-21 Updater lädt `main.zip` ohne Signatur/Pinning, Prüfsumme optional und aus demselben Archiv, kein Downgrade-Schutz. F-8-03
- R-22 Leaflet-CSS/Icons von unpkg und Kacheln von OSM: verstößt gegen CSP, gibt Kundenstandorte an Dritte; zwei widersprüchliche CSP-Header (Go vs. Caddy). F-7-02, F-8-10
- R-23 Caddy ohne Hostname → vermutlich kein Auto-TLS (bedingt, nicht live geprüft). F-4-06

## Mittel/Niedrig (Auswahl, Details in Teilberichten)
- Migrator ohne Advisory-Lock/Checksummen, Worker wartet nicht auf Migrationen (F-5-05); Listen ohne LIMIT, Pagination im Speicher (F-5-09, F-6-05); Trigram-Index für Kontaktsuche fehlt (F-5-10).
- Rohe `err.Error()` in HTTP-Antworten, teils in JSON-String konkateniert (`handlers/emails.go:203`) (F-6-06).
- River-Jobs ohne Uniqueness, Worker mit hartkodierter Fallback-DB-URL (F-6-07); SSE-Hub ohne Client-Limit/Heartbeat (F-6-08); `/api/v1/health` statisch (F-6-11); keine Healthchecks für Server/Worker.
- `react-leaflet` unter Hippocratic-2.1 (nicht permissiv), `river` MPL-2.0; `make check-licenses` ist grün, prüft aber nichts (F-8-05, F-8-09).
- Frontend: Query-Cache bei Logout nicht geleert, SSE-Events nie ausgewertet, 42 Mutationen mit nur 9 `onError`, 870-kB-Chunk (F-7-03..05).
- Backups 0644 und unverschlüsselt; Demo-Beispielwerte als Formular-Defaults.
- Doku-Drift: `/admin/ownership` (Doku) vs `/ownership` (Router); Karte geocodet nur 310 PLZ; Settings-Import wirkt in Docker nicht.
- Testlaufzeit sehr lang (auth 112 s, invite 111 s, handlers 141 s) → Pre-Push-Hook träge.
- Schwache Coverage: `cmd/server` 2 %, `internal/db` 10,4 %, `launcher/ui` 0 %, `cmd/worker` 0 Tests, `web/` 0 Tests; sqlc-Code 0 % (1122 Statements), Service/Handler/E2E laufen gegen In-Memory-Demo-Querier.

## Verifiziert OK
`go vet`, `gofmt`, `go test -race` (alle Pakete grün), `govulncheck` (0 aufgerufene Schwachstellen), `pnpm lint/typecheck/format:check/audit`, Migrations-Symlink intakt (00001–00021 lückenlos), `sqlc generate` ohne Diff. `UserResponse` mit sicheren Feldern, Mail-DTO ohne Passwort, Refresh-Rotation und Invite-Annahme atomar, Body-Limits, Server-Timeouts, Graceful Shutdown, CSV-Formel-Schutz, KB-Queries ohne Embedding-Vektor.

## Regression gegenüber Vorgänger-Reviews
Mehrere als "behoben" dokumentierte Punkte sind im Code nicht wirksam: Audit-Hash-Kette (SP8c), `token_version`-Invalidierung (SP8a §3.4), PII-Guard-Abdeckung laut `AI_COPILOT_AND_COMPLIANCE.md` (SP8c), Backup-Restore (durch SP8c gebrochen). Das deutet auf Merge ohne Postgres-Integrationstest (R-19) als systemische Ursache.

## Fix-Backlog (eigene Folgepläne)

1. **Plan A — Release-Blocker & Security (S1/S2):** R-01 (neue Migration, die `notes`-Schema korrekt indiziert; 00010 nie editieren, stattdessen Pfad für bereits angewendete Installationen klären), R-02/R-03 (SSE `SendToUser`, Mail-Scoping), R-04 (partielle Updates/COALESCE + Owner-Default), R-05 (Key auf Volume, Worker teilt Key), R-06 (Restore-Allowlist + Audit-Hash-Handling), R-07 (Launcher: Token/Origin/Host-Check), R-08 bis R-12, R-21.
2. **Plan B — Integrität:** R-13 bis R-16 (Audit-Kette mit Advisory-Lock/Transaktion, In-Memory-Fallbacks entfernen, IMAP streamen, Job-Uniqueness).
3. **Plan C — Tests & CI:** Postgres-Service in CI, golangci-lint (Toolchain-Pin), Diff-Coverage mit `-base`, RBAC-Matrix-Test für alle Routen, Worker-/Launcher-/Web-Tests (Vitest), Migrations-Replay-Test auf leerer DB.
4. **Plan D — Compliance/Features:** R-17, R-18 (Einwilligungsprüfung, Löschen/Export, PII-Guard für DE-PII, `WEBHOOK`-Freigabe), Doku korrigieren.
5. **Plan E — Hygiene:** R-22, R-23, Health/Readiness, Lizenzen, Listen-Limits, Fehlerantworten, Frontend-Punkte, Doku-Drift.

Empfohlene Reihenfolge: C (Postgres-CI zuerst, sonst bleiben A/B ungeprüft) parallel zu A; dann B, D, E.

## Live-Verifikation (2026-10-01)

Aufbau: frisch gebauter `crm-server` (HEAD c6bac3e) gegen eine leere, wegwerfbare `pgvector/pgvector:pg16`-Instanz (Port 55432), `DEMO_MODE=false`. Admin- und BENUTZER-Account per Invite angelegt, Requests per curl, Zustand per SQL geprüft. `data/`, `backups/` und bestehende Docker-Volumes blieben unberührt, Container danach entfernt. Probe-Tests für Guard und Launcher liefen über `go test -overlay` (keine Datei im Repo geändert).

| ID | Ergebnis | Beleg |
| --- | --- | --- |
| R-01 | **BESTÄTIGT** | Server bricht auf leerer DB ab: `failed executing migration 00010_foreign_key_indexes.sql: ERROR: column "contact_id" does not exist (SQLSTATE 42703)`. Mit `00010` als angewendet markiert laufen 00011–00021 durch. |
| R-02 | **BESTÄTIGT** | `GET /events/stream` als BENUTZER empfängt `note.created` mit Inhalt der Admin-Notiz und `appointment.created` des Admin-Termins. `GET /contacts/{id}` desselben Kontakts liefert für BENUTZER 403. |
| R-03 | **BESTÄTIGT** | BENUTZER liest Betreff und Body einer Mail im privaten Admin-Postfach über `/emails/messages` und `/emails/threads/{id}` und setzt Tags per `PATCH /emails/{id}/tags` (200). `/emails/accounts` ist korrekt gescopt (`[]`). |
| R-04 | **BESTÄTIGT** | `PUT /deals/{id}` (200) setzt `contact_id` und `assigned_to` auf NULL; `value` bleibt, wenn es fehlt. |
| R-05 | statisch bestätigt | Compose mountet `crm_keys` nur auf `/app/keys` (JWT-Key). Der Secrets-Key liegt per Default in `data/secrets.key` (relativ zu `/app`) und ist nicht persistiert; der Worker hat kein Key-Volume. Nicht live (Image-Rebuild) getestet. |
| R-06 | **BESTÄTIGT** | `POST /backup/drill` → 500 `audit_logs did not survive the round trip (11 source rows vs 11 scratch rows)`. `POST /backup/restore` mit dem eigenen Export → 400 `table "audit_logs" does not have a column "prev_hash"` in beiden Modi (merge, replace); nichts committet. |
| R-07 | **BESTÄTIGT (Lesepfade)** | Launcher antwortet bei `Origin: evil.example` und `Host: evil.example` auf `/api/status`, `/logs`, `/versions`, `/backups`, `/settings/export` mit 200 → kein Auth-/Origin-/Host-Schutz, DNS-Rebinding möglich. Schreibende Routen nicht ausgeführt (Docker-Zugriff), aus dem Code gleich ungeschützt. |
| R-08 | **BESTÄTIGT, Ursache klar** | `POST /auth/refresh` und `/auth/logout` → 403 `csrf_token_invalid`, auch mit gültigem Cookie/Header; `POST /contacts` mit demselben Token → 201. Ursache: beide Routen haben `CSRFProtectionMiddleware` ohne Auth-Middleware, `claims` ist nil, `ValidateCSRFToken(token, "")` schlägt immer fehl (`internal/auth/csrf.go`). Folge: Logout widerruft nichts, Refresh funktioniert nie, Sitzung endet mit Access-Token-Ablauf (~30 min). |
| R-09 | teilweise | BENUTZER POST `/contacts` → 201 (Schreibrecht ohne Permission-Check). Ownership greift: fremder Deal GET/PUT → 403, Löschen fremder Kontakt → 403, `/users` und `/backup/export` → 403. |
| R-10 | **BESTÄTIGT, schlimmer als gedacht** | Rollenwechsel: `token_version` bleibt 0. Nutzer auf `DEACTIVATED`: altes Access-Token liest (200) **und schreibt** (`POST /contacts` 201) weiter bis Ablauf; nur neue Logins werden mit 401 blockiert. |
| R-12 | **herabgestuft S2 → S3** | Compose-Default ist leer (zufälliges Passwort, Datei 0600), `must_change_password=true` wird gesetzt. Rest-Risiko: bekanntes Beispielpasswort in `.env.example`. |
| R-13 | **BESTÄTIGT** | 30 parallele `POST /contacts`: 12 → 42 Audit-Zeilen, **6 Verzweigungen** (mehrere Zeilen mit identischem `prev_hash`). |
| R-14 | nicht live auslösbar | Bei DB-Ausfall greift vorher die Session-Prüfung (401). Notizen werden bei laufender DB normal persistiert; Fallback-Pfad nur aus dem Code belegt. |
| R-18 | **BESTÄTIGT** | Guard-Probe: maskiert IBAN, Kreditkarte, Steuer-ID. **Nicht** maskiert: Name, Adresse, Telefon (`+49 171 …`), E-Mail, Geburtsdatum. `Ignore previous instructions. system: …` unverändert. |
| R-17 | statisch | `consent_*` wird nur im Kontakt-Service gelesen/geschrieben, nirgends in E-Mail-, Telefonie- oder Automation-Code geprüft. `POST /telephony/calls` protokolliert nur Anrufe (kein Wählen). |
| R-09/R-15/R-16/R-19–R-23 | nicht live | Code-/Konfigurationsbefunde, siehe Teilberichte. |

**Neue Befunde aus dem Live-Test (nicht in den Teilberichten):**
- **N-01 (S1):** `POST /deals` liefert auf echter Postgres **immer 500** (`null value in column "value" … violates not-null constraint`). `CreateDealRequest` hat kein `value`-Feld, der Handler übergibt `Value` nie. Das Kanban kann auf Postgres keine Deals anlegen; Demo/E2E (In-Memory-Querier) verdecken das. Das Frontend (`DealsPage.tsx:160`) nutzt diesen Endpoint.
- **N-02 (S3):** `POST /notes` akzeptiert nicht existierende `entity_id` (`0000…`) und beliebige `type`-Werte (`BOGUS_TYPE`) mit 201; `notes` hat keinen FK und keine Validierung.
- **N-03 (S3):** `PUT /deals/{id}` erwartet `value` als String, Zahl liefert 400; ohne `value` bleibt der alte Wert. API-Vertrag undokumentiert/inkonsistent.
- **N-04 (S4):** Access-Token-Laufzeit ~30 min, Verlängerung durch R-08 unmöglich.

**Aktualisierte Release-Blocker:** R-01 bis R-08, R-10, R-13, N-01 sind live belegt.

## Offene Verifikation (nach Live-Test)
Nicht live geprüft: R-05 (Image-Rebuild), schreibende Launcher-Routen (R-07), R-11, R-14 bis R-16, R-19 bis R-23 (CI, TLS/Caddy, Updater), EXPLAIN der Kernqueries, `-race -count=3` der großen Pakete, `golangci-lint` (Toolchain).
