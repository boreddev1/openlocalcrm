# 02 – Testabdeckung und Testqualität (Review 2026-09-30)

Stand: Commit c6bac3e, Arbeitsbaum sauber. Read-only Review, Task 2 des Plans `docs/superpowers/plans/2026-09-30-full-source-review-plan.md`.
Datenbasis: Baseline des Controllers im Scratchpad (`cov.out`, `cov-func.txt`, `test.log`; `go test` mit `-coverprofile`, **pro Paket**, ohne `-coverpkg`), eigene `go test -race -count=3` Läufe auf kleinen Paketen, Code-Lektüre der Tests, `scripts/coverage.sh`, `scripts/diff-coverage.go`, `.githooks/pre-push`, `.github/workflows/ci.yml`, `playwright.config.ts`, `e2e/*.spec.ts`.

Wichtiger Methodenhinweis: Das Baseline-Profil misst nur Statements des jeweils getesteten Pakets. Funktionen, die ausschließlich aus anderen Paketen heraus getestet werden (z. B. `auth.RequirePermission` über `internal/server`-Tests, `crypto.EncryptSecret` über `email`/`auth`-Tests), erscheinen als 0,0 %. Jede unten genannte Lücke wurde deshalb zusätzlich per `grep` über alle `*_test.go` auf Testaufrufer geprüft; nicht so geprüfte Zahlen sind als "Paketprofil" gekennzeichnet.

## 1. Kennzahlen

| Messung | Wert |
|---|---|
| Gesamt, Paketprofil, alle Statements | 59,7 % (9844 von 16479 Statements; `go tool cover -func` total: 59,6 %) |
| davon sqlc-Code `internal/db/*.sql.go` | 0 von 1122 Statements (0,0 %, 176 Funktionen, 6,8 % aller Statements) |
| Gesamt ohne `*.sql.go` | 64,1 % (9844 von 15357) |
| Gesamt ohne sqlc, Demo-Querier, Launcher, `scripts/`, `web/embed.go`, `cmd/setup-launcher`, `models.go` ("Produktlogik") | 72,7 % (6163 von 8478) |
| Funktionen < 50 % (ohne `*.sql.go`) | 232 von 920; davon 102 im `internal/db/demo/querier.go` (Testdouble) und 37 in Launcher/Setup-Launcher |
| Race-Läufe (eigene Ausführung, `-race -count=3`) | `sse`, `storage`, `crypto`, `connectors`, `queue`, `mailclient`, `geocoding`, `core/notification`, `core/telephony`: alle ok, keine Races/Flakes |
| Web (`web/src`, 34 TS/TSX-Dateien) | 0 Unit-Tests, kein Vitest/Jest in `web/package.json` (nur `test:e2e`) |
| E2E | 18 Spec-Dateien (ca. 44 Tests, 1923 Zeilen), laufen gegen `DEMO_MODE=true` (siehe F-2-06) |

Paketabdeckung (Baseline, `test.log`):

| Paket | Abdeckung | Paket | Abdeckung |
|---|---|---|---|
| `cmd/server` | 2,0 % | `core/notification` | 89,7 % |
| `cmd/setup-launcher` | 22,6 % | `core/reports` | 91,8 % |
| **`cmd/worker`** | **0,0 % (keine Tests)** | `core/telephony` | 84,2 % |
| `internal/ai` | 74,7 % | `core/todo` | 51,3 % |
| `internal/auth` | 77,9 % | `internal/crypto` | 62,6 % |
| `internal/connectors` | 72,7 % | `internal/db` | 10,4 % |
| `core/appointment` | 59,5 % | `internal/db/demo` | 58,2 % |
| `core/audit` | 86,5 % | `internal/geocoding` | 100 % |
| `core/automation` | 78,0 % | `internal/launcher` | 61,5 % |
| `core/backup` | 84,8 % | `internal/launcher/ui` | 0,0 % |
| `core/company` | 77,4 % | `internal/mailclient` | 79,6 % |
| `core/contact` | 68,0 % | `internal/queue` | 58,1 % |
| `core/deal` | 58,4 % | `internal/server` | 94,2 % |
| `core/email` | 57,4 % | `internal/server/handlers` | 70,0 % (141 s Laufzeit) |
| `core/emailtemplate` | 92,0 % | `internal/sse` | 86,5 % |
| `core/export` | 92,3 % | `internal/storage` | 83,0 % |
| `core/invite` | 86,4 % | `scripts`, `web` | 0,0 % |
| `core/note` | 54,9 % | | |

## 2. Bekannte Nullstellen (Step 2)

| Bereich | Stand | Was ist ungetestet | Schlimmster Fehler |
|---|---|---|---|
| `cmd/worker` (104 Zeilen) | 0 Tests; `main`, `aiEnv` 0 % | DB-Retry, River-Start/-Stop, Graceful Shutdown, Env-Fallbacks, Verdrahtung der Dienste (`cmd/worker/main.go:22-104`) | Worker startet ohne Jobs oder ohne Auslieferung der Mails/Workflows, ohne dass ein Test es merkt; Standard-DB-URL mit Klartext-Credentials im Code (`main.go:~36`, Task 3/8) |
| `internal/queue` | 58,1 %; `NewClient` 0 %, `ClientHolder.Insert` 0 % | Client-Aufbau, Periodic-Jobs-Registrierung (`client.go:41-54`), Retry/Backoff/Max-Attempts (nicht konfiguriert, River-Default), Insert-Fehlerpfade | Jobs werden nie eingeplant (Mail-Sync, Inaktivitäts-Scan); Fehler beim Enqueue von Triage werden nur geloggt (`jobs.go:136-139`) |
| `internal/connectors` | 72,7 %, 1 Testdatei (174 Zeilen), `IngestLead` 65 % | Duplikat-/Fehlerpfade beim Lead-Import, Token-Vergleich im Handler (`handlers/connectors.go:66`) | Lead-Verlust oder Doppelanlage bei Webhook-Wiederholung |
| `internal/mailclient` | 79,6 %, 608 Zeilen Tests mit In-Process-Server | TLS-Fehlerpfade/Timeouts nur teilweise | akzeptabel; Restrisiko: Zertifikatsprüfung (Task 4) |
| `internal/storage` | 83 %; `BaseDir` 0 %, `Delete` 71 % | Symlink-/Windows-Pfadfälle im Test nicht belegt (nur 90 Zeilen Tests) | Path-Traversal; Paket hat aktuell keinen Produktivnutzer (F-1-12) |
| `internal/crypto` | 62,6 %, 70 Zeilen Tests | `GetOrCreateMasterKey` 66 % (Dateirechte, Schlüsselverlust), `EncryptSecret`/`DecryptSecret` im Paketprofil 0 % (nur indirekt über `email` und `auth` getestet, dort Roundtrip-Asserts vorhanden: `email/service_test.go:167,229`, `auth/service_test.go:292-293`) | Verschlüsselung von TOTP-Geheimnissen und Mail-Passwörtern fällt bei Schlüsselproblemen unbemerkt aus; kein Test für manipulierten Ciphertext/falschen Schlüssel |
| `web/` | 0 Unit-Tests, kein Test-Runner | `api/client.ts` (446 Zeilen: CSRF, Refresh-Lock, 401-Handling), `AuthContext.tsx`, Kanban-DnD, Formulare | Redirect-Schleifen/Token-Refresh-Fehler und fehlerhafte optimistische Updates nur via E2E gegen Demo-Backend sichtbar |
| `cmd/server` | 2,0 %, nur `bootstrap_test.go` | `loadOrGenerateKey`, `bootstrapAdminUser`, `main` 0 % | Fehler bei JWT-Schlüssel-Erzeugung oder Bootstrap-Admin-Passwortdatei (Sicherheitsrelevant) ungetestet |
| `internal/launcher/ui`, `scripts/`, `web/embed.go` | 0 % | UI/Tooling | niedrig |

## 3. Funktionsgenaue Lücken nach Risiko (Step 1)

Quelle: `cov-func.txt`, Funktionen < 50 %, ohne `*.sql.go`, ohne Demo-Querier/Launcher. Nach Querprüfung auf Testaufrufer:

| Risiko | Funktion | Paketprofil | Querprüfung |
|---|---|---|---|
| Auth/Session | `auth.Service.RevokeAllSessions` (`auth/service.go:153`), `WithSessionRevoker` | 0 % | kein Test in `internal/auth`; Handler-Test `session_test.go:19-61` nutzt Ersatzimplementierung -> echte Massenwiderrufs-Logik (Token-Version hochzählen + SSE-Kick) ungetestet |
| Auth/Session | `auth.RevokeRefreshToken`, `HashPasswordLimited` | 0 % | Refresh-Revoke in `service_test.go` getestet; `HashPasswordLimited` ohne Test |
| Auth/HTTP | `handlers/auth.go`: `ChangePassword`, `SetupTOTP`, `VerifyTOTP`, `DisableTOTP` (0 %), `Refresh` (19 %) | 0-19 % | Service-Ebene getestet (`auth/service_test.go`), HTTP-Handler (Status-Codes, Cookie-Setzen, Rate-Limit) nur über E2E mit `page.route`-Mocks (`auth.spec.ts:118-175`) |
| Zugriffsschutz | `auth.RequirePermission` (`rbac.go:109`), `RequireRole/RequireAnyRole` | 0 % | indirekt über `server/rbac_routes_test.go`, der nur 10 Routen prüft (F-2-05) |
| CSRF | `server/csrf.go` `SetCSRFCookie`/`ClearCSRFCookie`, `auth/csrf.go` `SetCSRFSecret`/`SetCSRFCookie` | 0 % | Middleware getestet (`server/csrf_test.go`), Cookie-Attribute (Secure/SameSite) nicht; doppelte Implementierung (Task 3) |
| SSE | `sse.Hub.SendToUser` (`hub.go:47`) | 0 % | kein Test; dieser Pfad ist die einzige Nutzer-Isolation (F-1-02) |
| Migration | `db.RunMigrations`, `GetMigrationStatus`, `db.WithTx`, `RunTx` | 0 % | kein Test; Migrator-Verhalten bei parallelem Start server+worker ungeprüft (Task 5) |
| Backup | `backup.scratch.Prepare` 9,5 %, `skipJSONValue` 27,8 % | niedrig | Restore-Kernpfad teils nur in Postgres-Tests (werden übersprungen, F-2-03) |
| KI/SSRF | `ai.ResearchCompany` 31,7 %, `scrapeWebsite` 38,6 %, `GetRecentLogs` 31 %, `Record` 41,7 % | niedrig | `isPrivateOrReservedIP` hat Test (`research_test.go:10`), Transport/Redirect-Schutz im Scrape-Pfad nur teilweise |
| Mail | `core/email.SendMessage` 40,9 %, `handlers/emails.go` `UpdateAccount` 40 %, `DeleteAccount` 40,7 %, `TestAccountConnection` 43,8 %, `SyncAccount` 45,5 % | 40-46 % | Fehlerpfade (SMTP-Fehler, fremde Konten) nicht abgedeckt |
| Einwilligung/Import | `contact.Service.Search` 0 %, `SetOwner` 0 % (Contact/Company), `OptionalBool.UnmarshalJSON` 0 % | 0 % | `Search` ist der Suchpfad der UI (`/contacts?q=`); `OptionalBool.UnmarshalJSON` validiert die Einwilligungsfelder (`core/contact/service.go:34-74`) |
| Domänenlogik | `deal.ListByStage` 0 % (nur Nil-Safety-Test), `deal.mergeCustomFields` 26 %, `deal.classifyMutationError` 0 %, `todo.Update` 0 %, `note.List` 35 %, `appointment.GetByID` 37,5 % | niedrig | Kern-CRUD meist nur Happy-Path (siehe Abschnitt 4) |

## 4. Testqualität, Stichprobe (Step 3)

Gelesen bzw. gezielt geprüft: `core/todo/service_test.go`, `handlers/todos_test.go`, `handlers/ownership_scoping_test.go`, `handlers/ownership_test.go`, `server/rbac_routes_test.go`, `server/csrf_test.go`, `db/queries/sp8a_test.go`, `core/deal`/`core/contact`/`auth`-Testdateien per Kopf/`grep`.

Stärken:
- Ownership-Lesescoping wird domänenübergreifend mit echten Router-Handlern geprüft (`ownership_scoping_test.go:28-250`: BENUTZER sieht nur Eigenes, 403 bei fremder ID, ADMIN sieht alles) für Kontakte, Firmen, Deals, Todos, Termine, Notizen.
- `server/csrf_test.go` prüft Fehlerfälle (fehlender/falscher Header, Bearer-Bypass, Webhook-Ausnahme).
- 104 Testdateien nutzen `t.Parallel`; keine Skips ohne Begründung; keine `test.fixme`.

Schwächen:
1. **Alle Service-/Handler-/Router-Tests laufen gegen `demo.InMemoryQuerier`** (z. B. `todo/service_test.go:15-18`, `handlers/todos_test.go:21-23`, `server/rbac_routes_test.go:37-40`). Das ist eine handgeschriebene Nachbildung der SQL-Logik (102 Funktionen < 50 % im Profil); SQL-Filter für Owner/Scoping, Constraints und Transaktionen werden nie gegen PostgreSQL geprüft (F-2-03).
2. Happy-Path-Lifecycle-Tests mit einer Test-Funktion pro Domäne und wenig Assertions: `core/todo/service_test.go` (1 Funktion, 74 Zeilen, `Update` nicht aufgerufen), `core/note` (74 Zeilen, Abdeckung 54,9 %), `core/company` (79 Zeilen), `core/notification`, `core/telephony` (58 Zeilen), `handlers/companies_test.go`, `handlers/todos_test.go` (je ca. 90 Zeilen, ADMIN-Rolle, keine Fehlerfälle/Ownership-Schreibzugriffe). Keine `t.Run`-Tabellentests in diesen Dateien.
3. Ownership-Scoping wird nur für **Lesezugriffe** domänenübergreifend getestet; Schreib-/Löschpfade (PUT/DELETE fremder Todos, Deals, Notizen, Termine) sowie Telefonie, Mail, Reports, Export sind nicht im Scoping-Test. `ownership_test.go` prüft nur Kontakte (Reassign-Regeln).
4. `db/queries/sp8a_test.go:16-41` prüft Migrationen per String-Vergleich auf SQL-Text (`"ALTER TABLE users ADD COLUMN token_version ..."`), nicht per Ausführung. Schützt vor Löschen der Zeile, nicht vor falschem Verhalten.
5. Ersatzimplementierungen statt Produktlogik in Handler-Tests (`session_test.go`) und fehlende Negativtests für Fehlerformat/Fehlerinterna.
6. Zeitabhängige Tests (Flaky-Risiko): `time.Sleep` in `auth/limiter_test.go:47,102` (110-210 ms Fenster), `sse/hub_test.go:30,40` (50 ms), `core/contact/service_test.go:215,353` (10 ms), `launcher/server_test.go` (3), `ai/gateway_test.go`, `db/invites_templates_test.go`. Eigene Läufe von `sse` mit `-race -count=3` waren stabil; `auth` (112 s), `launcher`, `db`, `demo`, `server` wurden aus Laufzeitgründen nicht wiederholt.
7. Die kompletten RBAC-Tests decken nur 10 Routen ab (`rbac_routes_test.go:117-173`), obwohl ca. 100 Routen existieren (F-2-05).

## 5. Race-/Flaky-Lauf (Step 4)

Ausgeführt: `go test -race -count=3` für `internal/sse`, `internal/storage`, `internal/crypto`, `internal/connectors`, `internal/queue`, `internal/mailclient`, `internal/geocoding`, `internal/core/notification`, `internal/core/telephony` -> alle `ok`, keine Races. Nicht ausgeführt (Vorgabe: Baseline-Lauf nicht stören, lange Laufzeit): `./internal/...` komplett; insbesondere `internal/auth` (112 s), `core/invite` (111 s), `core/emailtemplate` (62 s), `db/demo` (81 s), `server` (73 s). Diese Pakete haben Nebenläufigkeit (Limiter, SSE, Sessions) und sollten im Controller-Lauf mit `-race` bestätigt werden. Auffällig: lange Laufzeiten einzelner Pakete (über 100 s bei `auth`, `invite`) deuten auf echtes Argon2-Hashing ohne Test-Parameter hin (Task 3 prüft).

## 6. E2E-Abdeckung gegen Feature-Matrix (Step 5)

Stand: `playwright.config.ts:23-35` startet `DEMO_MODE=true ./bin/crm-server` (In-Memory-Querier, kein Postgres, kein Caddy) plus `pnpm dev` (Vite-Devserver :5173), nicht das eingebettete Produktions-Bundle.

| Domäne | Spec | Bewertung |
|---|---|---|
| Kontakte, Firmen, Deals, Todos, Notizen, Kalender+Telefonie, Inbox, Automationen, Reports, Export+Notifications, KI, Connectors, Angebotsrechner, Backup | vorhanden | ok, nur Demo-Backend |
| Karte | `map-view.spec.ts` | 1 Test mit 3 Assertions (Container und Attribution sichtbar), keine Marker-/Koordinaten-Prüfung |
| 2FA | `auth.spec.ts:118-175` | TOTP-Endpunkte werden per `page.route` **gemockt**, der echte Flow (Secret, Verify, Replay) wird nicht ausgeführt |
| Einladungen/Nutzerverwaltung | nur Teilbezug in `auth.spec.ts` | kein durchgängiger Test Einladung -> Accept -> Login -> Rollenwechsel |
| E-Mail-Templates | keine | fehlt |
| Eigentümer-Admin (Bulk-Assign) | keine (auch keine UI) | fehlt |
| SSE | keine (`EventSource`/`events/stream` nicht in `e2e/`) | fehlt; Nutzer-Isolation ungeprüft (F-1-02) |
| Einstellungen Export/Import | keine | fehlt |
| `parse-bill`, Sessions `logout-all` | `logout-all` nur in `auth.spec.ts` erwähnt; `parse-bill` keine | teilweise/fehlt |
| CSRF/RBAC im Browser | `auth.spec.ts`, `live-simulation.spec.ts` | teilweise |

Spec-Hygiene: keine `test.skip`/`fixme`; ein `waitForTimeout(500)` in `backup-stream.spec.ts:107` (negative Prüfung, dort vertretbar, aber flaky-anfällig). Dateien mit sehr wenigen Assertions: `map-view.spec.ts` (3), `notes-activity-log.spec.ts` (7), `notifications-and-export.spec.ts` (7).

## 7. Gate-Wirksamkeit (Step 6)

- `scripts/coverage.sh:71` ruft `go run ./scripts/diff-coverage.go -coverprofile=... -min=...` **ohne** `-base`. `scripts/diff-coverage.go` vergleicht dann per Default mit `git diff HEAD` (Z. ~20-33); im Pre-Push-Hook (nach dem Commit, sauberer Arbeitsbaum) ist dieser Diff leer und das Programm beendet sich mit "Keine Änderungen an Go-Dateien im Diff gefunden" und Exit 0 (Z. 48). Die Diff-Coverage (50 % auf neuem Code, AGENTS.md Invariante 6) wird damit im Standardablauf nie wirksam geprüft.
- Gesamt-Gate: `coverage.sh:39` misst mit `-coverpkg=./...` (Cross-Package), `go tool cover -func | grep total` zählt sqlc-generierten Code (0 von 1122 Statements), Demo-Querier, Launcher-UI und `scripts/` mit, während der Diff-Teil `.sql.go` ausschließt (`diff-coverage.go:207`). Ergebnis: das Gesamtmaß ist um ca. 4,5 Prozentpunkte nach unten verzerrt (59,6 % gegenüber 64,1 % ohne sqlc) und die zwei Teilgates nutzen verschiedene Grundgesamtheiten.
- CI (`.github/workflows/ci.yml:38-39`): `go test -race -v ./...` ohne Coverage, ohne Diff-Gate, ohne `golangci-lint`; das Coverage-Gate existiert nur im lokalen Pre-Push-Hook (per `--no-verify` oder fehlendes `make setup-hooks` umgehbar). Lokal läuft das Gate ohne `-race`, CI ohne Coverage: keine Stelle führt beides aus.
- Kein PostgreSQL-Service in CI; `TEST_DATABASE_URL` wird nirgends gesetzt außer `scripts/test-backup-integration.sh` (manuell). Daher werden folgende Tests in CI und Pre-Push übersprungen: `auth/session_postgres_test.go:26`, `core/backup/scratch_postgres_test.go:37`, `core/invite/postgres_test.go:49`, `handlers/ownership_postgres_test.go:21`, `ai/live_ollama_test.go:26-29`.

## 8. Findings

### F-2-01 (S2) Diff-Coverage-Gate ist im Pre-Push wirkungslos
- Beleg: `scripts/coverage.sh:71` (kein `-base`), `scripts/diff-coverage.go` Default `git diff HEAD` bei `baseRef == ""`; Pre-Push läuft nach dem Commit -> leerer Diff -> Exit 0.
- Auswirkung: Die Regel "neue Zeilen >= 50 % abgedeckt" wird de facto nicht erzwungen; ungetestete Sicherheitslogik kann ungehindert gepusht werden.
- Fix: im Hook `-base "$(git merge-base origin/main HEAD)"` (bzw. `@{upstream}`) übergeben, Test des Gate-Tools mit Fixture-Diff, Fehler bei leerem Diff mit Commits im Bereich.
- Aufwand: S.

### F-2-02 (S2) CI erzwingt weder Coverage noch golangci-lint
- Beleg: `.github/workflows/ci.yml:26-45` (gofmt, `go vet`, `go test -race`, Lizenzen), `ci.yml:85-95` (Web). Kein Schritt für `scripts/coverage.sh`, `golangci-lint`, `gosec` oder `govulncheck`, obwohl `AGENTS.md` `make check` als kanonisches Gate nennt.
- Auswirkung: Gates nur lokal und umgehbar; Regressionen (z. B. Coverage-Einbruch, `errcheck`-Funde) fallen erst im Review auf.
- Fix: CI-Job `make check` äquivalent (golangci-lint, coverage mit `-base origin/main`, `govulncheck`), Branch-Protection auf diesen Job.
- Aufwand: S-M.

### F-2-03 (S2) Kein Test gegen echtes PostgreSQL in CI; SQL-Schicht faktisch ungetestet
- Beleg: sqlc-Code 0 von 1122 Statements (`cov-func.txt`, alle `*.sql.go`); Service-/Handler-/Router-Tests nutzen `demo.NewInMemoryQuerier()` (`core/todo/service_test.go:15`, `handlers/todos_test.go:21`, `server/rbac_routes_test.go:37`); Postgres-Tests werden ohne `TEST_DATABASE_URL` übersprungen (`auth/session_postgres_test.go:26`, `handlers/ownership_postgres_test.go:21`, `core/invite/postgres_test.go:49`, `core/backup/scratch_postgres_test.go:37`); `ci.yml` definiert keinen `services: postgres`; `db/queries/sp8a_test.go:16-41` prüft nur SQL-Text.
- Auswirkung: Ownership-Filter in SQL (IDOR-Klasse, Review-Fokus 1), Constraints, Transaktionen, Migrations-Reihenfolge und Hash-Kette (`00021`) werden nie real ausgeführt; Demo-Querier und echte Queries können unbemerkt divergieren (besonders riskant, weil E2E ebenfalls nur Demo nutzt).
- Fix: CI-Service `pgvector/pgvector:pg16`, `TEST_DATABASE_URL` setzen, Postgres-Tests in CI erzwingen (Skip nur lokal); gemeinsame Vertragstests (`querier`-Suite), die gegen In-Memory und Postgres laufen; Ownership-Matrix als Postgres-Test.
- Aufwand: M-L.

### F-2-04 (S3) Gate-Berechnung verzerrt und zu weich
- Beleg: `scripts/coverage.sh:39-49` (total inkl. `.sql.go`, Demo-Querier, `scripts/`), `diff-coverage.go:207` schließt `.sql.go` nur im Diff aus; Schwellen 50 %/50 % (`pre-push`), kein Paketminimum. Sicherheitskritische Pakete haben nur 55-78 % (`core/note` 54,9, `core/deal` 58,4, `internal/crypto` 62,6, `internal/auth` 77,9), `cmd/server` 2 %, `cmd/worker` 0 %.
- Auswirkung: Gesamtwert sagt wenig über Risikopfade; gute Abdeckung in Demo-/Launcher-Code kompensiert Lücken in Auth/Worker.
- Fix: generierten Code und Testdoubles aus dem Profil ausschließen (`-coverpkg` mit Filter oder nachträgliches Filtern), Paketminima (z. B. `auth`, `ai`, `audit`, `crypto`, `storage`, `queue`, `server/handlers` >= 75 %), Baseline per `go test -race` messen.
- Aufwand: S-M.

### F-2-05 (S2) Rollen-/Berechtigungstest deckt nur 10 von ca. 100 Routen
- Beleg: `internal/server/rbac_routes_test.go:117-173` (Users, Backup-Export, Reports, Export-CSV, Telefonie, Automationen GET/POST, AI-Observability, Deal-Delete, Notifications). Nicht enthalten: Kontakte/Firmen/Todos/Notes/Appointments-Schreibzugriffe, `/emails/*` inkl. `send`, `/ai/*` (außer Observability), `/settings/*`, `/ownership/*`, `/email-templates/*`, `/backup/restore`, `/import/contacts`, `/calculator/solar`. Gleichzeitig sind sechs Berechtigungen ungenutzt (F-1-07).
- Auswirkung: Die in F-1-07 beschriebene Lücke (BENUTZER darf schreiben/mailen/löschen) wird von keinem Test erkannt.
- Fix: Test generiert das Route-Inventar aus `chi.Walk` und verlangt für jede Route eine erwartete Statusmatrix je Rolle (Whitelist-Ansatz: neue Route ohne Eintrag = Testfehler).
- Aufwand: M.

### F-2-06 (S3) E2E prüfen nur das Demo-Backend und mocken Sicherheitsflüsse
- Beleg: `playwright.config.ts:23-35` (`DEMO_MODE=true`, Vite-Devserver), `e2e/auth.spec.ts:46,118-175` (`page.route` für Passwort-Änderung, `/me`, TOTP setup/verify/disable).
- Auswirkung: Produktionspfade (Postgres, Secure-Cookies hinter Caddy, eingebettetes Bundle, echte Sessions/TOTP) sind nie Ende-zu-Ende getestet; Mock-Tests bestätigen nur das UI, nicht die Serverlogik.
- Fix: zweiter Playwright-Lauf gegen `docker compose` (Postgres + Caddy + Prod-Bundle) für Auth/2FA/Invite/Ownership; TOTP-Test mit echtem Code-Generator statt Mock.
- Aufwand: M-L.

### F-2-07 (S3) `cmd/worker`, `internal/queue` und `cmd/server` Bootstrap ohne Verhaltenstests
- Beleg: `cmd/worker/main.go` 0 %, `queue.NewClient` 0 % (`queue/client.go:20`), `cmd/server/main.go` `loadOrGenerateKey`/`bootstrapAdminUser`/`main` 0 %.
- Auswirkung: Start-/Shutdown-, Retry- und Schlüssel-/Admin-Bootstrap-Fehler (sicherheits- und betriebskritisch) werden nicht erkannt; Periodic-Jobs (Mail-Sync 60 s, Inaktivitäts-Scan) könnten stillschweigend nicht laufen.
- Fix: Worker-Verdrahtung in testbare `run(ctx, cfg)`-Funktion auslagern; Integrationstest mit Postgres und River (Job wird ausgeführt, Retry bei Fehler, sauberer Shutdown); Tests für Key-Erzeugung (Dateirechte 0600) und Bootstrap-Datei.
- Aufwand: M.

### F-2-08 (S3) Lücken bei Sicherheits- und Risikopfaden (funktionsgenau)
- Beleg: Abschnitt 3: `RevokeAllSessions`/`WithSessionRevoker` ohne Test, `SendToUser` ohne Test, `RunMigrations`/`GetMigrationStatus` ohne Test, HTTP-Handler für TOTP/Passwort/`Refresh` (19 %) nur über Mock-E2E, `contact.Search` 0 %, `OptionalBool.UnmarshalJSON` (Einwilligungsvalidierung) 0 %, E-Mail-Versand-/Konto-Handler 40-46 %, `crypto` ohne Negativtests (manipulierter Ciphertext, falscher Schlüssel). E-Mail-Templates, SSE, Settings-Import und Ownership-Admin ohne E2E (Abschnitt 6).
- Auswirkung: Widerruf von Sitzungen, Nutzerisolation in SSE und Migrationslauf sind Sicherheits-/Betriebsgrundlagen ohne Regressionsschutz.
- Fix: gezielte Tests je Funktion (Tabellentests inkl. Fehlerpfade), priorisiert: Session-Revoke (Postgres), SSE-Isolation, Migrator (Parallelstart), TOTP-Handler, Einwilligungs-JSON, crypto-Negativfälle.
- Aufwand: M.

### F-2-09 (S3) Web-Frontend ohne Unit-Test-Gerüst
- Beleg: `web/package.json` (nur `test:e2e`), 34 Quelldateien, 0 Testdateien; `api/client.ts` 446 Zeilen (CSRF, Cross-Tab-Refresh-Lock Z. ~50-110, 401-Handling), `AuthContext.tsx` 127 Zeilen.
- Auswirkung: Refresh-/CSRF-/Fehlerlogik und Kanban-Rollback sind nur über langsame Demo-E2E abgesichert; ESLint meldet bereits 2 `exhaustive-deps`-Warnungen (`OfferCalculatorModal.tsx:117`, `SimulationWidget.tsx:288`).
- Fix: Vitest + Testing Library + MSW einführen; zuerst `api/client.ts` (Refresh-Lock, 401, CSRF-Header), `AuthContext`, `DealsPage`-DnD-Rollback, `CSVImportModal`.
- Aufwand: M.

### F-2-10 (S3) Flaky-Risiko durch zeitbasierte Tests und ungeprüfte Race-Läufe
- Beleg: `time.Sleep` in `auth/limiter_test.go:47,102`, `sse/hub_test.go:30,40`, `core/contact/service_test.go:215,353`, `launcher/server_test.go` (3), `ai/gateway_test.go`, `db/invites_templates_test.go`; Race-Lauf der großen Pakete (`auth`, `invite`, `server`, `demo`, `launcher`, `db`) in dieser Review nicht wiederholt (Laufzeiten 60-112 s).
- Auswirkung: sporadische CI-Fehler auf langsamen Runnern (Rate-Limiter-Fenster 110 ms) verleiten zum Ignorieren roter Läufe.
- Fix: injizierbare Uhr (`now func() time.Time`) in Limiter/Hub-Tests, `Eventually`-Polling statt `Sleep`; Argon2-Parameter in Tests senken; `-race -count=3` als nächtlichen Job.
- Aufwand: M.

### F-2-11 (S4) Schwache Assertions in Happy-Path-Lifecycle-Tests
- Beleg: `core/todo/service_test.go:15-74` (eine Funktion, `Update` fehlt, Liste nur `len > 0`), `handlers/todos_test.go:21-89` (nur ADMIN, kein Fehlerpfad), `core/note` (74 Zeilen, 54,9 %), `core/company` (79 Zeilen), `core/telephony` (58 Zeilen), `handlers/companies_test.go` (92 Zeilen, 1 Test-Funktion).
- Auswirkung: Die statistisch "ok" wirkenden 51-77 % verbergen fehlende Negativ-/Berechtigungsfälle.
- Fix: Tabellentests mit Rollen (ADMIN/VERTRIEB/BENUTZER fremd/eigen), Validierungsfehler, Nicht-gefunden, Konflikte; Owner-Schreibzugriffe (PUT/DELETE) ergänzen.
- Aufwand: M.

## 9. Priorisierte Test-Backlog-Empfehlung (Eingabe für Task 9)

1. Gate reparieren (F-2-01, F-2-02, F-2-04) – klein, sofort wirksam.
2. Postgres in CI + Vertragstests + Ownership-/RBAC-Vollmatrix (F-2-03, F-2-05) – deckt die wahrscheinlichste Fehlerklasse (IDOR) ab.
3. Tests für F-1-01/F-1-02/F-1-03 als Regressionstests, bevor diese Findings behoben werden (DB-Fehler -> kein stiller Fallback; SSE-Isolation; Einwilligungs-Sperre).
4. Worker/Queue-Integrationstest, Session-Revoke, Migrator (F-2-07, F-2-08).
5. Vitest-Gerüst und echter 2FA-/Invite-E2E (F-2-09, F-2-06).
