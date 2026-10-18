# 08 – Build, CI, Launcher, Lieferkette, Compliance

Stand: 2026-09-30, read-only. Belege als `datei:zeile` oder Command-Output. Aufwand: S = <0,5 Tag, M = 1-2 Tage, L = >2 Tage.

Nicht ausgeführt (laut Vorgabe): `go test ./...`, Container-Start, Backup-/Restore-Livetests, `govulncheck`/`gosec` (nicht installiert/freigegeben), `make check` (enthält Docker-Backup-Integration). Ausgeführt: `make check-licenses` (grün, siehe F-8-08), `go list -m -u all`, `pnpm licenses list`, `docker compose config` (ohne `DB_PASSWORD` korrekt Fehler "DB_PASSWORD must be set in .env"; mit gesetztem Wert gültig).

## Findings

### F-8-01 (S1) Frische Installation scheitert voraussichtlich an Migration 00010 (Index auf nicht existierende Spalte `notes.contact_id`)
- Evidenz: `internal/db/migrations/00010_foreign_key_indexes.sql:17` (`CREATE INDEX ... ON notes (contact_id)`); `notes` wird nur in `00008_notes_schema.sql:2-11` angelegt, und zwar mit `entity_type, entity_id, ...`, ohne `contact_id`; `grep -n "notes" internal/db/migrations/*.sql` zeigt später nur `00020:24` (`ADD COLUMN user_id`). Der Migrator bricht beim ersten Fehler ab und rollt zurück (`internal/db/migrator.go:151-161`, kein Sonderfall für 00010 in `migrator.go:106-165`). Bestätigt wird das durch den Kommentar in `scripts/test-backup-integration.sh:22-27` ("00010 is NOT executed ... indexes notes(contact_id), a column that no migration ever creates, so replaying the historical migration set from an empty database cannot work. Recording it as pre-applied ...").
- Auswirkung: `make up` auf leerem Postgres-Volume endet laut Code in `failed executing migration 00010_foreign_key_indexes.sql`; der Server startet nicht. Bestehende Installationen (00010 bereits angewendet) bemerken nichts. Nicht live reproduziert (Container-Start untersagt); Querverweis Task 5.
- Warum unentdeckt: CI hat keinen Postgres-Service und fährt E2E im Demo-Modus (F-8-06).
- Fix: neue Migration darf 00010 nicht heilen (angewendete Migrationen nie ändern, AGENTS.md Invariante 3), aber 00010 ist auf Neuinstallationen nie angewendet; Optionen: (a) 00010 zu `CREATE INDEX IF NOT EXISTS ... ON notes (entity_type, entity_id)`-neutral machen, da sie auf keiner Produktivinstanz je lief (Entscheidung Maintainer, Abweichung von Invariante dokumentieren), oder (b) Migrator um Baseline-Logik erweitern. Dazu CI-Job mit `pgvector/pgvector:pg16`-Service, der Migrationen von Null anwendet.
- Aufwand: M

### F-8-02 (S1) Launcher-HTTP-API ohne Authentifizierung, Origin- oder Host-Prüfung (CSRF/DNS-Rebinding gegen localhost)
- Evidenz: `internal/launcher/server.go:43-68` registriert u. a. `/api/admin/reset-password`, `/api/update/execute`, `/api/backup/restore`, `/api/backup/upload`, `/api/settings/import`, `/api/settings/rebuild`, `/api/control/`, `/api/install/` ohne Middleware; `grep -n "Origin\|Sec-Fetch\|r.Host" internal/launcher/server.go` liefert keinen Treffer; Handler dekodieren Body ohne Content-Type-Prüfung (`server.go:485-489` `json.NewDecoder(r.Body).Decode`); Bindung an `127.0.0.1:<port>` (`cmd/setup-launcher/main.go:82`), GUI-Port 9099 (`cmd/setup-launcher/cli.go:164`).
- Auswirkung: jede im Browser des Anwenders geöffnete Webseite kann per `fetch('http://127.0.0.1:9099/api/admin/reset-password', {method:'POST', mode:'no-cors', body: '{"password":"..."}'})` (einfacher Request, kein Preflight) den Admin-Passwort-Reset, Restore (Datenersatz), Update-Ausführung oder Container-Kontrolle auslösen. Dazu DNS-Rebinding, da kein Host-Header-Check. Ergebnis bei Reset: Übernahme des Admin-Kontos. Nicht praktisch getestet, aus Code abgeleitet; der Port ist bekannt (9099, Fallback +50).
- Fix: zufälliges Sitzungs-Token (beim Start in die geöffnete URL eingebettet, als Header/Cookie verlangt), `Host`-Allowlist (`127.0.0.1:<port>`, `localhost:<port>`), `Origin`/`Sec-Fetch-Site` prüfen, nur `Content-Type: application/json` akzeptieren.
- Aufwand: M

### F-8-03 (S2) Updater zieht `main`-Branch-ZIP ohne Signatur/Pinning; Prüfsummenprüfung ist optional und aus derselben Quelle
- Evidenz: `internal/launcher/updater.go:556` (`zipURL := ".../archive/refs/heads/main.zip"`), kein Tag, kein Commit-Pinning, keine Signatur; `updater.go:353-397` `VerifyBinaryChecksum`: fehlende Prüfsummendatei -> `return nil` (Z. 356-358), fehlender Eintrag -> `return nil` (Z. 391-393); `updater.go:434-455` sucht `SHA256SUMS` innerhalb des entpackten Archivs, also dieselbe Vertrauensquelle wie die Binary; das Release-`checksums.txt` (`.github/workflows/docker-publish.yml:~140`, `sha256sum * > checksums.txt`) wird vom Updater gar nicht verwendet. `updater.go:606-610`: Fehler beim Binary-Tausch wird nur als "Hinweis" geloggt ("fahre fort"). `ApplyStagedFiles` überschreibt `docker-compose.yml`, `Caddyfile`, Skripte im Projektverzeichnis (`updater.go:287-345`, Schutzliste `IsProtectedPath` nur für `.env`, `backups/` etc.).
- Auswirkung: Wer `main` kompromittiert (oder ein ungeprüfter Merge) verteilt sofort Compose-/Caddy-Dateien und ggf. Binaries an alle Installationen; Downgrade-/Versionsschutz existiert nicht (Vergleich nur `isNewerCommit` per String, `updater.go:192-205`). `io.Copy` der ZIP ohne Größenlimit (`updater.go:574`), Entpacken ohne Gesamt-/Dateigrößenlimit (Zip-Bomb, `updater.go:228-285`; Zip-Slip ist geschützt, `:233,:249`).
- Fix: Updates nur aus signierten Release-Assets (Tag) laden; Prüfsumme/Signatur (cosign/minisign) gegen separat geliefertes, eingebettetes Schlüsselmaterial prüfen; fehlende Prüfsumme = Abbruch; Größenlimits; Versionsvergleich per SemVer mit Downgrade-Sperre.
- Aufwand: L

### F-8-04 (S2) CI deckt nicht alle Gates aus AGENTS.md ab
- Evidenz `.github/workflows/ci.yml`: vorhanden sind gofmt (Z. 24), `go vet`, `go test -race`, Lizenz-Skript, `pnpm audit/format:check/lint/typecheck/build`, Playwright. Es fehlen: `golangci-lint` (kein Treffer in `Makefile`, `.github`, `.githooks`, `scripts`; `.golangci.yml` ist toter Code), Coverage-Gate 50 %/50 % (nur lokal im pre-push: `.githooks/pre-push`, `scripts/coverage.sh:46`; nicht in CI), Backup-/Invite-SQL-Integration gegen echtes Postgres (`make check-backup-integration` nur lokal), Prüfung Migrations-Symlink/Verzeichnisgleichheit, `sqlc generate` + `git diff --exit-code`, `gosec`/`govulncheck`, Docker-Build auf PRs, Secret-Scan (nur pre-commit). `make test` läuft ohne `-race` (`Makefile:69`), CI mit `-race`.
- Auswirkung: Hooks lassen sich mit `--no-verify` umgehen, dann gibt es auf `main` keine Erzwingung der dokumentierten Gates.
- Fix: fehlende Schritte in `ci.yml`, Postgres-Service (pgvector) für Integrationstests.
- Aufwand: M

### F-8-05 (S2) Lizenz-"Audit" prüft keine Abhängigkeiten
- Evidenz: `scripts/check-licenses.sh:14-23`: greppt nur in der statischen, handgepflegten `THIRD-PARTY-LICENSES.csv` nach Copyleft-Namen und führt `go list -m all > /dev/null` aus (Ergebnis wird verworfen); `go-licenses` wird in CI installiert (`ci.yml:~38`), aber nie aufgerufen. `make check-licenses` endet daher immer grün ("All dependencies ... 100% compliant"). Die CSV ist veraltet: nennt `pgx` 5.10.0 und `river` 0.44.1, `go.mod:9,11` hat 5.11.0 bzw. 0.47.0; enthält `tailwindcss`/`vite` (Dev), aber nicht `qrcode.react`, `@dnd-kit/accessibility`, `@react-leaflet/core`, `cookie`, `js-tokens`, `loose-envify` (`pnpm licenses list --prod`); Go-Test-Abhängigkeit `stretchr/testify` ist nicht gelistet (direkter Eintrag in `go.mod:13`, Vergleich `comm` CSV vs. `go.mod`). `docs/COMPLIANCE_AND_LICENSES.md:38-39` behauptet, das Skript "scannt alle Abhängigkeiten und generiert" die CSV: falsch.
- Auswirkung: Die in AGENTS.md geforderte Invariante 7 ("100 % permissiv, geprüft") ist nicht maschinell abgesichert.
- Fix: `go-licenses report ./... ` bzw. `go-licenses check --allowed_licenses=...` und `pnpm licenses list --prod --json` gegen Allowlist; CSV generieren statt pflegen.
- Aufwand: S

### F-8-06 (S3) E2E-Tests laufen nur gegen den In-Memory-Demo-Modus
- Evidenz: `scripts/run-e2e.sh` (Zeile "Running E2E tests against Vite frontend & Go demo backend"); `ci.yml` ohne `services: postgres`; `docker-compose.demo.yml:30` `DEMO_MODE=true`.
- Auswirkung: SQL-, Migrations-, Ownership- und Restore-Pfade werden in CI nie gegen Postgres geprüft (Ursache für F-8-01).
- Fix: zusätzlicher E2E-/Integrationsjob mit echter DB.
- Aufwand: M

### F-8-07 (S3) Supply-Chain-Härtung der Pipelines fehlt
- Evidenz `ci.yml`/`docker-publish.yml`: Actions nur per Tag (`actions/checkout@v4`, `docker/build-push-action@v7`, `softprops/action-gh-release@v3`, `pnpm/action-setup@v6`), nicht per Commit-SHA; `ci.yml` ohne `permissions:` (Standard-Token-Rechte des Repos); `docker-publish.yml:17-19` `contents: write` schon für den Image-Job (nur `packages: write` nötig); `pnpm` `version: latest` (`ci.yml:~60`, `docker-publish.yml:~96`) und `go install github.com/google/go-licenses@latest` (`ci.yml:~38`) -> nicht reproduzierbar; Node 20 in CI vs. `node:22-alpine` im Dockerfile (`Dockerfile.server:6`) vs. `pnpm@9`. Kein SBOM, keine Provenance/Attestation, keine Image-Signatur (cosign), kein Schwachstellen-Scan. Images werden bei jedem Push auf `main` veröffentlicht und als `latest` getaggt (`docker-publish.yml:4-7,~41`), ohne dass `ci.yml` bestanden sein muss (kein `needs`/`workflow_run`). Release-Binaries werden ohne Testlauf gebaut (`release-binaries` hängt nur an `build-and-push`).
- Fix: SHA-Pinning (Dependabot/Renovate), `permissions: contents: read` Default, `docker/build-push-action` mit `provenance: true`, `sbom: true`, cosign-Signatur, Release nur nach grüner CI.
- Aufwand: M

### F-8-08 (S3) Container-Images nicht reproduzierbar / teils veraltet
- Evidenz: `Dockerfile.server:16` und `Dockerfile.worker:1` `golang:alpine` (ungepinnt) mit `ENV GOTOOLCHAIN=auto` (lädt beliebige Toolchain beim Build aus dem Netz); Laufzeit-Image `alpine:3.20` (`Dockerfile.server:23`, `Dockerfile.worker:10`; nach Alpine-Release-Plan Ende des Supports April 2026, aus dem Gedächtnis, bitte prüfen); `caddy:2-alpine`, `pgvector/pgvector:pg16` als bewegliche Tags (`docker-compose.yml:5,30-`). Positiv: Non-Root (UID 10001), kein Port der DB am Host.
- Fix: Digest- oder Patch-Tags, `golang:1.26-alpine`, `GOTOOLCHAIN=local`, `-trimpath`.
- Aufwand: S

### F-8-09 (S3) Lizenzpolitik in AGENTS.md vs. tatsächliche Abhängigkeiten
- Evidenz: `pnpm licenses list --prod`: `react-leaflet` und `@react-leaflet/core` = **Hippocratic-2.1** (`web/node_modules/react-leaflet/package.json` `"license": "Hippocratic-2.1"`). Die Hippocratic License enthält Nutzungsbeschränkungen (Ethik-/Menschenrechts-Klausel), ist nicht OSI-zertifiziert und damit keine "permissive" Lizenz im Sinn von AGENTS.md ("MIT/Apache/BSD/etc., 100 % permissiv"); `scripts/check-licenses.sh:10` führt sie dennoch als erlaubt auf. `river` (+ `riverdriver*`, `rivershared`, `rivertype`) = **MPL-2.0** (`THIRD-PARTY-LICENSES.csv`), schwaches Datei-Copyleft; in `docs/COMPLIANCE_AND_LICENSES.md:32` als erlaubt ("dynamisch verlinkt") deklariert, bei Go-Static-Linking unpassende Formulierung, praktisch unkritisch solange MPL-Dateien unverändert bleiben und der Quelltext verfügbar ist.
- Auswirkung: Rechtliche Klärung nötig, vor allem wenn das Produkt an Kunden verteilt wird (Launcher-Binaries werden als Release verteilt).
- Fix: Policy explizit ergänzen (MPL-2.0 zulässig; Hippocratic entweder freigeben mit Begründung oder `react-leaflet` ersetzen/`leaflet` direkt nutzen); Hinweis auf Quelltextverfügbarkeit für MPL-Komponenten in `COMPLIANCE_AND_LICENSES.md`.
- Aufwand: S

### F-8-10 (S3) Doppelte, widersprüchliche CSP (Go und Caddy)
- Evidenz: `internal/server/router.go:110` (`script-src 'self' 'unsafe-inline'`, `img-src 'self' data:`, `frame-ancestors 'none'`) und `Caddyfile:22` (`script-src 'self'`, `img-src 'self' data: https:`, `object-src 'none'`, `base-uri 'self'`, kein `frame-ancestors`). Browser wenden beide Header an (Schnittmenge); zusätzlich setzt Go `X-Frame-Options`, `Referrer-Policy` etc. doppelt zu Caddy (`router.go:106-109`, `Caddyfile:12-18`). Go erlaubt `'unsafe-inline'` für Skripte, obwohl der Vite-Build keinen Inline-Skript braucht (`web/dist/index.html` hat 1 `<script type=module src>`). Folge für Karte siehe F-7-02.
- Fix: eine Quelle der Wahrheit (Go-App-Layer, Caddy nur HSTS), `script-src 'self'` ohne `unsafe-inline`.
- Aufwand: S

### F-8-11 (S3) Caddy: `DOMAIN` ist wirkungslos; "Auto-TLS" nicht belegt
- Evidenz: `Caddyfile:6` Site-Adresse `:80, :443` (kein Hostname); `grep -n DOMAIN Caddyfile docker-compose.yml` leer, obwohl `docs/CONFIGURATION_AND_ENV.md:12` ("Hauptdomain für Caddy-TLS & Cookie-Domain") und `.env.example:8` `DOMAIN` dokumentieren. Caddy stellt für Adressen ohne Hostnamen keine öffentlichen Zertifikate automatisch aus (Verhalten laut Caddy-Doku, nicht ausprobiert). `Strict-Transport-Security ... preload` (`Caddyfile:16`) wird auch über HTTP :80 gesendet.
- Auswirkung: Produktiv-HTTPS nicht out of the box; Cookies mit `Secure` hängen an `X-Forwarded-Proto`/`FORCE_SECURE_COOKIES` (`auth.go isSecureRequest`), der Betrieb über reines HTTP im LAN sendet Cookies ohne `Secure`.
- Fix: `{$DOMAIN}` im Caddyfile verwenden und `DOMAIN` in der Compose-`environment`-Sektion des Caddy-Services durchreichen; `preload` nur bei bestätigtem HTTPS.
- Aufwand: S

### F-8-12 (S3) Backups/Dumps unverschlüsselt und teils weltlesbar
- Evidenz: `internal/launcher/engine.go:647,652` `os.WriteFile(backupPath, ..., 0644)` (auch `:568,:602`); lokales Verzeichnis `backups/` enthält `openlocalcrm_backup_*.sql` (je ~60 kB, `ls -la backups`, Dateirechte `-rw-r--r--`); `.gitignore` schließt sie aus (`/backups/`, `*.sql`) und `git ls-files` listet sie nicht. `docs/DEPLOYMENT.md:84-121` kennt nur unverschlüsselte Dumps/JSON-Exporte (`/api/v1/backup/export` liefert Klartext-JSON mit allen Tabellen).
- Auswirkung: Kundendaten (PII, Aufbewahrungspflichten) liegen unverschlüsselt und mit 0644 auf Platte; Löschfristen für alte Backups nicht definiert (F-8-15).
- Fix: Rechte 0600, Verzeichnis 0700; optionale Verschlüsselung (age/AES-GCM); Aufbewahrungsdauer/Rotation dokumentieren.
- Aufwand: S

### F-8-13 (S3) Keine Healthchecks für Server/Worker, schwache Start-Reihenfolge
- Evidenz: `docker-compose.yml`: nur `db` hat `healthcheck` (Z. 84-89); `server`/`worker`/`caddy` ohne; `caddy depends_on: server` ohne Bedingung (Z. 13-14), `worker depends_on server: service_started` (Z. 66-67). `docs/DEPLOYMENT.md:69-77` nennt `/api/v1/health` als Liveness, im Compose aber nicht genutzt; `alpine` ohne `curl`/`wget` -> Healthcheck bräuchte `wget` (BusyBox vorhanden) oder Binary-Subcommand. Der Server führt Migrationen beim Start aus (`migrator.go`), Worker kann vor fertigem Schema starten.
- Fix: `healthcheck` mit `wget -qO- http://127.0.0.1:8080/api/v1/health`, `depends_on: condition: service_healthy`; Worker wartet auf Server-Health.
- Aufwand: S

### F-8-14 (S3) Dokumentierte Aussagen zu KI-Compliance nicht oder nur teilweise durch Code gedeckt
Abgleich `docs/AI_COPILOT_AND_COMPLIANCE.md` (Aussage -> Befund):

| Aussage im Dokument | Befund | Beleg |
|---|---|---|
| PII-Guard maskiert IBAN, Kreditkarte, Secrets, Steuer-ID vor jedem LLM-Aufruf | Teilweise. Guard wird im Gateway auf Prompt und System-Prompt angewendet (`internal/ai/gateway.go:159-160,296`). Maskiert werden nur IBAN (nur Ziffernformat, `guard.go:9`), genau 16-stellige Kartennummern ohne Luhn (`guard.go:11`; Amex/15-stellig nicht), Steuer-ID/USt-ID, Secret-Muster. **Namen, Anschriften, Telefon, E-Mail, Zählernummern (`zaehlernummer`), Verbrauchsdaten und Freitext werden nicht maskiert.** Die Überschrift "DSGVO & PII-Filter" überschätzt die Wirkung. | `internal/ai/guard.go:9-20,30-55` |
| Doku-Codeausschnitt/Platzhalter `[REDACTED_CARD]`, `[REDACTED_TAXID]` | Veraltet: Code nutzt `[REDACTED_CREDIT_CARD]`, `[REDACTED_TAX_ID]`, zusätzlich `[REDACTED_VAT_ID]`; `SanitizeInput` ruft `SanitizeInputWithCount` | `guard.go:43-55` |
| "100 % On-Premise, Kein Datenabfluss" (Ollama) | Nur bei `AI_PROVIDER=ollama`. Gateway unterstützt `openai`, `mistral`, `nebius` mit `AI_API_KEY`/`AI_BASE_URL` (`gateway.go:53-54`, `.env.example:39-45`). Kein Hinweis auf Auftragsverarbeitungsvertrag, Drittlandübermittlung oder Opt-in im Doku-Text. | `internal/ai/gateway.go`, `.env.example` |
| "Jede KI-Antwort ist als KI-unterstützt gekennzeichnet (Generiert mit Gemma 12B)" | Teilweise/irreführend: Kennzeichnung sind statische Texte im UI (`ActivityLogDrawer.tsx:238`, `OfferCalculatorModal.tsx:196`, Simulation). Der Modellname wird nicht aus der Laufzeitkonfiguration übernommen; bei anderem Modell/Provider ist der Hinweis falsch. Eine durchgängige Kennzeichnung in Chat/Triage ist nicht belegt. | `web/src/components/notes/ActivityLogDrawer.tsx:238` |
| "Keine KI-Entscheidung führt selbsttätig zu ... Mail-Versendungen. Jeder Schritt verharrt im Status WAITING_APPROVAL" | Nur `DRAFT_EMAIL` pausiert mit `WAITING_APPROVAL`. `SET_TAG`, `CREATE_TASK`, `NOTIFY_USER`, `WEBHOOK` laufen automatisch (`automation/service.go:198-213,357,408,451,522`); `WEBHOOK` sendet Daten an externe URLs ohne Freigabe. Die Aussage "jeder Schritt" ist falsch; für E-Mail-Entwürfe gilt sie. | `internal/core/automation/service.go:198-213,522` |
| "Zeitstempel, Token-Anzahl, Latenz, Modell werden für jede Inferenz unveränderbar protokolliert" | Felder vorhanden (`prompt_tokens`, `completion_tokens`, `latency_ms`, `model_name`, `provider`, `pii_redactions_count`, `human_approved`, `created_at`), aber **nicht unveränderbar**: `ai_audit_logs` hat weder Trigger noch Hash-Kette (Migration 00021 ergänzt nur `user_id`; die Kette betrifft `audit_logs`). Keine Aufbewahrungs-/Löschregel. | `internal/db/migrations/00005_ai_observability_and_research.sql:3-15`, `00021_audit_hash_chain.sql:3-9` |
| "Audit-Trail für Zeitstempel und Herkunft der Einwilligung" (UWG) | Zeitstempel `consent_updated_at` ja; Herkunft/Quelle/Wortlaut der Einwilligung: keine Spalte (`00004_energy_d2d_and_consent.sql:6-8`). Nachweisbarkeit nach § 7 UWG/DSGVO Art. 7 (Nachweis der Einwilligung) daher nur über allgemeines `audit_logs` (Task 4 prüfen). | `00004_energy_d2d_and_consent.sql:6-8` |
| "14-tägige Fristerfassung bei Verbraucherverträgen" (§ 355 BGB) | Nicht im Code belegt: es gibt nur `widerrufen_at` und `widerruf_grund` (`internal/core/deal/service.go:384-399`); keine Fristberechnung, Erinnerung oder Validierung der 14 Tage. Nur UI-Text `DealsPage.tsx:299`. | `internal/core/deal/service.go:384-410` |
| `COMPLIANCE_AND_LICENSES.md:1.1`: "audit_logs ... unveränderbar" | Hash-Kette existiert (`00021`, Spalten `prev_hash`, `hash`), aber keine DB-seitige Sperre für UPDATE/DELETE (`grep TRIGGER/REVOKE` in Migrationen: kein Treffer); Manipulation wird erkennbar, nicht verhindert (Task 4 prüft Verifikation). | `internal/db/migrations/00021_audit_hash_chain.sql` |
| `COMPLIANCE_AND_LICENSES.md:1.1`: "PII-Guards maskieren vor dem Verlassen des Systems" | Siehe Zeile 1; zusätzlich Webhook-/Connector-Ausgänge nicht erfasst. | `gateway.go` |

- Fix: Doku korrigieren (Umfang des Guards, Provider-Abhängigkeit, Approval nur für `DRAFT_EMAIL`), oder Funktionen ergänzen (Namen/Telefon/E-Mail-Maskierung per Kontakt-Lookup, unveränderlicher `ai_audit_logs`, Einwilligungsquelle, Widerrufsfrist).
- Aufwand: M

### F-8-15 (S3) Kein Lösch-/Aufbewahrungskonzept, kein Verzeichnis von Verarbeitungstätigkeiten, kein AV-Hinweis
- Evidenz: `grep -rniE "löschkonzept|retention|aufbewahr|Verzeichnis von Verarbeit|AV-Vertrag|Auftragsverarbeit" docs/*.md README.md` -> keine Treffer; im Code keine Retention-/Purge-Jobs für Audit-, AI-Audit-, Session-, Mail- oder Backup-Daten (`grep` nach `retention|purge` in `internal/**/*.go`, SQL: kein Treffer). `docs/COMPLIANCE_AND_LICENSES.md:11` nennt als Betroffenenrecht nur Art. 20 (Export `contacts.csv`); Art. 15/16/17/18/21 nicht beschrieben (technische Abdeckung: siehe Task 1 Step 5).
- Auswirkung: DSGVO-Pflichtdokumentation für Betreiber fehlt; bei Nutzung externer LLM-Provider fehlt der Hinweis auf AV-Vertrag/Drittlandtransfer; Audit-Tabellen wachsen unbegrenzt (RAM-/Plattenbudget).
- Fix: Betreiber-Vorlage (VVT, TOM, Löschfristen pro Tabelle), Retention-River-Job, AV-Hinweis in `AI_GATEWAY.md`.
- Aufwand: M

### F-8-16 (S3) SECURITY.md und Versionsangaben inkonsistent
- Evidenz: `SECURITY.md:9-10` unterstützt nur `1.0.x`; Health/App melden `3.0.0` (`internal/server/router.go:139-140`, `web/package.json:4`), Launcher-Fallback `v0.9`/`v1.0.1` (`internal/launcher/config.go:95,237`); Meldeadresse `security@openlocalcrm.local` ist keine zustellbare Domain; Zusage "Bestätigung innerhalb von 48 Stunden" ohne Prozess. `docs/DEPLOYMENT.md:38` nennt Migrationen "00001 bis 00011", real 21 (`ls internal/db/migrations`).
- Fix: Versionsmatrix, echte Kontaktadresse/Advisory-Link, Doku aktualisieren.
- Aufwand: S

### F-8-17 (S3) Launcher schreibt dauerhaft über `0644`-Dateien, Standard-Admin-Passwort im Beispiel
- Evidenz: `.env.example:12` `INITIAL_ADMIN_PASSWORD=change_this_secure_admin_password_123!` (bekanntes Beispiel-Passwort, wenn die Datei unverändert kopiert wird: `DEPLOYMENT.md:19`), `.env.example:19-21` zusätzlich Passwort doppelt in `DATABASE_URL`; `docs/DEPLOYMENT.md:48` Beispiel `${DB_PASSWORD:-crm_pass}` mit Default-Passwort. Positiv: Launcher schreibt `.env` mit 0600 (`internal/launcher/config.go:353`), Compose erzwingt `DB_PASSWORD` (`docker-compose.yml:79`).
- Auswirkung: Wird `.env.example` unverändert kopiert, entsteht ein Admin mit öffentlich bekanntem Passwort; ob der Server das abweist und `must_change_password` setzt, gehört zu Task 3.
- Fix: Platzhalter leer lassen und Server-Start ohne Passwort verweigern bzw. Zufallspasswort erzeugen; Default in Doku entfernen.
- Aufwand: S

### F-8-18 (S4) Repo-Hygiene
- Evidenz `git ls-files | grep -E 'backups/|data/|coverage|\.log$|\.png$|\.out$|\.DS_Store|\.env'`: getrackt sind nur `.env.example`, `scripts/coverage.sh`, `scripts/diff-coverage.go` (Absicht) und `docs/superpowers/.DS_Store` (unerwünscht, trotz `.gitignore:.DS_Store`). Lokal vorhanden, aber korrekt ignoriert (`git check-ignore`): `openlocalcrm-setup.log` (20 kB, `grep -ciE "password|secret|token"` = 0, also keine Secrets), `coverage.out` (457 kB), `Screenshot 2026-09-18 at 15.31.27.png` (180 kB), `backups/*.sql`, `data/secrets.key`, `.env.backup.*` (0600). `.git` = 36 MB. `web/dist` ist getrackt (für `go:embed`; Build reproduziert identischen Hash), d. h. jeder Frontend-Build erzeugt ein ~850-kB-Diff. `.gitignore:49` `*.png` blockiert auch legitime Doku-Bilder. Veraltete Markenreste `mavalio-setup*.exe` (`docker-publish.yml`, `Makefile:39,44`).
- Fix: `git rm --cached docs/superpowers/.DS_Store`; Entscheidung zu `web/dist` (embed per CI-Build statt Commit) dokumentieren; Legacy-Alias-Dateien abkündigen.
- Aufwand: S

### F-8-19 (S4) Makefile-/Hook-Abweichungen von der Dokumentation
- Evidenz: `Makefile:76-77` `make lint` ohne golangci-lint (AGENTS.md: "golangci-lint"); `Makefile:79` `check` enthält `fmt` (verändert Dateien statt nur zu prüfen) und `setup-hooks` (setzt `core.hooksPath` als Nebeneffekt); `build-web` nutzt `pnpm install` statt `--frozen-lockfile` (`Makefile:14`); pre-commit Schritt 1b Secret-Scan nur 4 Muster (`.githooks/pre-commit`, private key, `sk-proj-`, `ghp_`, `re_`), keine Erkennung von z. B. AWS-/Slack-/generischen Hochentropie-Schlüsseln; pre-commit führt `go vet ./...` komplett bei jedem Commit mit Go-Dateien (langsam), bricht aber bei fehlendem Tool nur mit Warnung ab.
- Fix: golangci-lint in `lint`, separates `fmt-check`, `--frozen-lockfile`, gitleaks/trufflehog in CI.
- Aufwand: S

### F-8-20 (S4) Go-Module mit verfügbaren Updates
- Evidenz `go list -m -u all`: `golang.org/x/net` v0.58.0 -> v0.59.0, `golang.org/x/tools` 0.49.0 -> 0.50.0, `github.com/boombuler/barcode` (Pseudo-Version 2019) -> v1.1.0, `github.com/tidwall/pretty` 1.2.1 -> 1.2.2, `github.com/jackc/pgerrcode`, `kr/pretty`, `yuin/goldmark`, `x/xerrors` (alle Indirect/Test). Kein aufgegebenes Modul im direkten Abhängigkeitsbaum erkennbar; `boombuler/barcode` (transitiv über `pquerna/otp`) ist faktisch unmaintained (letzter Stand 2019, Pseudo-Version). `govulncheck` nicht ausgeführt.
- Fix: `govulncheck ./...` in CI, Dependabot.
- Aufwand: S

## Launcher-Checkliste (Task 8 Step 2)

| Frage | Ergebnis |
|---|---|
| Integrität der Downloads (Checksumme/Signatur) | Nur optional, gleiche Quelle, nicht signiert, kein Abbruch bei fehlender Datei: F-8-03 |
| Downgrade-Schutz | Nicht vorhanden (String-Vergleich Commit-SHA, `updater.go:192`) |
| Ausführung heruntergeladener Binaries | `HotSwapExecutable` ersetzt die laufende Datei (`updater.go:399-515`), Unix `chmod 0755` (`:469`), Windows Umbenennung `.old`; Quelle = `main.zip` |
| Rechteeskalation | Keine `sudo`-Automatik; bei fehlender Schreibberechtigung nur Hinweis (`updater.go:466`). Kein setuid. Positiv. |
| Zip-Slip | Geschützt (`..`/absolut abgewiesen, Präfixprüfung, `updater.go:233-251`) |
| Pfadbehandlung Windows/macOS | Geschützte Pfade/Ausnahmen über `IsProtectedPath` (`updater.go:207-226`); `.env`-Backup vor Überschreiben (`:320-334`) |
| `openlocalcrm-setup.log` | Ignoriert (`.gitignore:10`), nicht getrackt, keine Secrets-Treffer |
| Netzwerk-Angriffsfläche der GUI | F-8-02 |

## Deployment-Trockenlauf (Task 8 Step 6)

- `docker compose config`: ohne `DB_PASSWORD` Fehler wie erwartet; mit gesetztem Wert gültig, Ports nur für Caddy (80/443), DB/Server/Worker nur im Netz `crm-net`. Kein Container gestartet.
- Update-Pfad mit Migrationen: Server wendet beim Start in eigener Transaktion je Migration an (`migrator.go:126-162`); "Future versions" (DB neuer als Binary) wird erkannt (`migrator.go:170ff`, nicht tiefer geprüft). Fehler bei Migration 00010 auf leerer DB: F-8-01.
- **Backup/Restore-Live-Test: NICHT AUSGEFÜHRT** (Vorgabe). Durchzuführende Prozedur (für Folgearbeit): (1) `make up` mit Testdaten; (2) `GET /api/v1/backup/export` als Admin; (3) `docker compose down -v`, `make up` (leere DB, hier wird F-8-01 sichtbar); (4) `POST /api/v1/backup/restore?mode=replace` mit der Datei, Vergleich der Zeilenzahlen (`/api/v1/backup/drill`); (5) zusätzlich `pg_dump` -> `psql` in leere DB, anschließend Login, Ownership-Stichproben, Audit-Hash-Kette prüfen. Die vorhandene Integrationsprobe `scripts/test-backup-integration.sh` (Docker, `make check-backup-integration`) deckt Restore und Invite-SQL gegen pgvector ab, überspringt aber 00010 (siehe F-8-01), ist also kein Beleg für Frischinstallation.
- `docs/DEPLOYMENT.md:84-121`: Der "Wiederherstellungstest" (`psql ... < backup.sql` in die produktive DB ohne `--clean`) ist kein Drill und riskant; `tar ... /data/storage` adressiert einen Pfad im Container/Volume, nicht den Host.

## Geprüft ohne Befund (verified OK)

- `migrations` ist Symlink auf `internal/db/migrations` (`ls -l`: `migrations -> internal/db/migrations`); Pre-commit prüft Dateinamen-Gleichheit.
- Kein Secret im Git-Index (`git ls-files` ohne `.env`, Keys, Backups, `data/`); `.gitignore` deckt `.env*`, `*.key`, `*.pem`, `/data/`, `/backups/`, `*.log`, `coverage.*` ab; `.env.backup.*` haben 0600.
- `.env` wird vom Launcher mit 0600 geschrieben (`internal/launcher/config.go:353`); `DB_PASSWORD` im Compose Pflicht (`docker-compose.yml:79`).
- Docker: Non-Root UID 10001, Multi-Stage, `.dockerignore` schließt `.git`, `node_modules`, `bin`, `coverage` aus; DB-Port nicht am Host; `admin off` in Caddy (`Caddyfile:3`); `X-Real-IP`/`True-Client-IP` werden entfernt (`Caddyfile:30-31`), passend zu ClientIP-Logik.
- Frontend-Dependencies: `pnpm audit` sauber; Lockfile wird in CI und Dockerfile mit `--frozen-lockfile` genutzt.
- Release erzeugt `checksums.txt` (SHA-256, `docker-publish.yml`), CGO-frei, Ldflags mit Commit/Datum.
- Zip-Slip-Schutz im Updater vorhanden; Launcher bindet nur an `127.0.0.1` (`cmd/setup-launcher/main.go:82`).
- Keine GPL/AGPL/SSPL in `THIRD-PARTY-LICENSES.csv` und in `pnpm licenses list --prod` (Stichprobe der ausgegebenen Liste: MIT, ISC, BSD-2/3, Apache-2.0, MPL-2.0, Hippocratic-2.1; siehe F-8-09).
- Go-Gateway wendet den PII-Guard zentral auf Prompt und System-Prompt an (`internal/ai/gateway.go:159-160`) und validiert die Ausgabe (`gateway.go:238,282`); Skript-Tags und System-Prompt-Leaks werden entfernt (`guard.go:62-90`). Umfang siehe F-8-14.
