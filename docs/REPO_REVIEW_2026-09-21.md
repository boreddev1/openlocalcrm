# Repository-Review — OpenLocalCRM (2026-09-21)

> **Scope:** Review des gesamten Repository-Stands auf Main hinsichtlich (A) Repo-Hygiene &
> Open-Source-Konformität, (B) Dokumentationsqualität (Aktualität, Vollständigkeit, Struktur,
> Lesbarkeit) und (C) Code-Implementierung (Fehler, Dead Code, Sicherheit).
> **Methodik:** 3 unabhängige Codebase-Audits + manuelle Stichproben-Verifikation der zitierten
> Datei:Zeile-Angaben.
> **Status:** Reiner Befund-Report. Es wurden in diesem Schritt **keine Fixes angewendet** —
> Entscheidung über Umsetzung liegt beim Team.

---

## Executive Summary — kritischste Funde

Diese 6 Punkte zuerst, weil sie das größte Risiko bzw. den größten Vertrauensschaden für ein
Open-Source-Produkt darstellen:

1. **DB-Rollen-Modell widerspricht Doku und Code.** Die einzige Migration, die die Rolle
   definiert, legt `ENUM ('ADMIN', 'BENUTZER')` an (`internal/db/migrations/00001_initial_schema.sql:9`).
   README (`README.md:103`) und `router.go` (`internal/server/router.go:385,394-395`, Rollen
   `"BACKOFFICE"`/`"VERTRIEB"`) gehen jedoch von einem Drei-/Vier-Rollen-Modell
   (`ADMIN`, `VERTRIEB`, `BACKOFFICE`) aus. Ohne weitere Migration, die den Enum erweitert,
   würde das Zuweisen von `VERTRIEB`/`BACKOFFICE` an der DB scheitern. **Braucht Klärung durch
   Engineering, nicht nur Doku-Fix.**
2. **Dokumentierte Env-Variablen ohne Wirkung.** `.env.example` und
   `docs/CONFIGURATION_AND_ENV.md` nennen `JWT_PRIVATE_KEY_PATH`/`JWT_PUBLIC_KEY_PATH` und
   `STORAGE_LOCAL_DIR` — der Code liest aber `JWT_SECRET_KEY_PATH` und `STORAGE_PATH`
   (`cmd/server/main.go:126,130`). Wer `.env.example` folgt, konfiguriert wirkungslose Variablen.
3. **Öffentlich exponierter interner Audit-Report mit echten Default-Credentials.**
   `docs/AUDIT_REPORT_2026-09-21.html` ist getrackt und enthält u. a. `demo123` und den
   schwachen DB-Passwort-Default `crm_pass`. Nicht von README/anderen Docs verlinkt, aber für
   jeden Repo-Browser sichtbar.
4. **~77 MB kompilierte Binaries im Git-Tracking**, inkl. Alt-Branding-Dateien
   `bin/mavalio-setup.exe` / `bin/mavalio-setup-debug.exe` — bläht Klon-Größe und History aller
   Nutzer dauerhaft auf; gehört als Release-Artefakt (GitHub Releases), nicht ins Repo.
5. **Zwei sich überschneidende API-Dokus.** `docs/API.md` (246 Zeilen) und
   `docs/API_REFERENCE.md` (522 Zeilen) beschreiben dieselben Endpunkte in unterschiedlicher
   Tiefe; `API.md` ist von nirgends verlinkt (verwaist) und damit eine stille Zweitquelle, die
   veralten kann.
6. **Go-Versionsangabe an 3 Stellen falsch.** README-Badge (`README.md:3`: "Go 1.22 / 1.23"),
   README-Tabelle (`README.md:128-129`: "Go 1.22"), `docs/ARCHITECTURE.md:23-24` ("Go 1.23") —
   tatsächlich steht in `go.mod:3`: `go 1.26.0`.

---

## Kategorie A — Repo-Hygiene & Open-Source-Konformität

| Schweregrad | Fund |
|---|---|
| Kritisch | `docs/AUDIT_REPORT_2026-09-21.html` exponiert echte Democredentials (`demo123`) und Passwort-Default `crm_pass` öffentlich (s. Executive Summary #3). |
| Kritisch | `bin/*.exe` (~77 MB) inkl. `bin/mavalio-setup.exe`, `bin/mavalio-setup-debug.exe` im Git-Tracking, whitelisted in `.gitignore:10-11,21-22`. |
| Moderat | Durchgängige Alt-Branding-Reste "mavalio": `cmd/setup-launcher/dir.go:42-46` (hardcodierte Legacy-Pfade `C:\mavalio`, `%LOCALAPPDATA%\mavalio`), `internal/launcher/engine.go:274,281,304-308,319,349,411,575-646` (Legacy-Migrations-/Cleanup-Logik), `internal/launcher/config.go:162` (String-Match auf `"mavalio"`), `internal/db/demo/querier.go:501-502` (Legacy-Login-Alias `admin@mavalio.local`), plus mehrere Tests und `README.md:139,146,167,168,182,256-257`. |
| Moderat | `web/dist/assets/index-BuMGnBRO.css`, `web/dist/assets/index-mdyUyylu.js`, `web/dist/index.html` sind Build-Output und trotzdem getrackt — kein `.gitignore`-Eintrag für `dist/`. Risiko: veralteter Bundle wird ausgeliefert, falls `web/embed.go` dieses Verzeichnis statt eines frischen Builds einbettet. |
| Moderat | Fehlende Standard-OSS-Dateien: kein `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `CHANGELOG.md`. `.github/` hat dagegen bereits Issue-/PR-Templates, Dependabot und CI-Workflows — solide Basis, aber unvollständig. |
| Minor | `.github/.DS_Store` ist getrackt (macOS-Cruft in einem Verzeichnis, das eigentlich für CI-Konfiguration gedacht ist). |
| Minor | `docker-compose.yml:29,64` / `internal/launcher/templates.go:85`: `DATABASE_URL`-Default fällt auf schwaches Passwort `crm_pass` zurück, obwohl `db`-Service `DB_PASSWORD` korrekt über `:?`-Syntax erzwingt (`docker-compose.yml:76`) — inkonsistente Durchsetzung. |
| Minor | `THIRD-PARTY-LICENSES.csv` deckt Go- und Kern-Frontend-Deps ab, listet aber Dev-Deps wie `@types/*`, `eslint*`, `typescript`, `vite`-Tooling, `postcss`, `autoprefixer`, `prettier` nicht (geringes Risiko, da Dev-only). |
| Info (kein Fund) | Branding sonst konsistent: `go.mod:1` → `github.com/openlocalcrm/openlocalcrm`, `web/package.json:2` → `openlocalcrm-web`. LICENSE (MIT) korrekt und konsistent referenziert. |

---

## Kategorie B — Dokumentation

### Struktur / Auffindbarkeit

| Schweregrad | Fund |
|---|---|
| Kritisch | Quickstart in README erst ab Zeile ~135, nach Mermaid-Architektur-Diagramm (L17-58), vollständigem Feature-Katalog §1-§22 (L62-107), Security-Hardening (L110-119) und Resource-Tabelle (L123-131). Die für neue Nutzer wichtigste Info (Wie starte ich das?) liegt hinter ~120 Zeilen Marketing-/Architektur-Content. Kein Inhaltsverzeichnis für die 363-Zeilen-Datei. |
| Moderat | Innerhalb des Quickstart-Blocks selbst: "Option 1: Windows-Installer" (README.md:137-238, ~100 Zeilen mit Feature-Tabelle, Step-by-Step, Backup/Restore, Passwort-Reset, Troubleshooting) steht vor der eigentlich einfachsten, plattformunabhängigen "Option 2: Demo-Modus" (2-Zeilen-Docker-Befehl). |
| Minor | Statischer, zeitgestempelter Playwright-Testlauf-Output ist direkt ins README eingefügt (README.md:306-338, "27 passed"). Rottet beim nächsten Testadd/-umbenennen sofort zur Falschaussage — sollte nur als CI-Badge dargestellt werden, nicht als Textschnipsel. |
| Minor | Kaum Querverlinkung zwischen den `docs/`-Dateien (Hub-and-Spoke nur über README, keine lateralen Links, z. B. verweist API_REFERENCE.md nicht auf CONFIGURATION_AND_ENV.md für Token-Setup). |
| Minor | Inkonsistente TOC-Nutzung: GETTING_STARTED.md, FAQ_AND_TROUBLESHOOTING.md, USER_GUIDE_AND_PLAYBOOKS.md, END_TO_END_PROCESS_EXAMPLES.md haben funktionierende TOCs; ARCHITECTURE.md, API.md, API_REFERENCE.md, D2D_AND_CONNECTORS.md, AI_GATEWAY.md, AI_COPILOT_AND_COMPLIANCE.md, COMPLIANCE_AND_LICENSES.md, DEPLOYMENT.md haben keine. |

### Duplikate / Widersprüche

| Schweregrad | Fund |
|---|---|
| Kritisch | `docs/API.md` vs. `docs/API_REFERENCE.md` — identischer Titel, überlappender Endpunkt-Umfang, unterschiedliche Beispielformate. `API.md` ist von nirgends verlinkt (nur `API_REFERENCE.md` erscheint in `README.md:350`) → verwaiste Zweitquelle, Kandidat für Merge/Entfernen. |
| Moderat | `docs/API_REFERENCE.md`: Abschnittsnummerierung doppelt vergeben — "6. Notizen & KI-Synthese" (L276) und später erneut "6. Click-to-Call Telefonie" (L335); ebenso "7." zweimal (L310, L351). |
| Moderat | Rollen-Widerspruch: `docs/FAQ_AND_TROUBLESHOOTING.md:30-33` behauptet "genau zwei Rollen: Admin, Benutzer", während README/API_REFERENCE/Code drei+ Rollen beschreiben (siehe Executive Summary #1 — hier zusätzlich als Doku-internen Widerspruch sichtbar). |
| Minor | GETTING_STARTED.md-Quickstart und README-Quickstart (Option 2/3) sind nahezu wortgleich dupliziert (gleiche Befehle, Creds, URLs) — README könnte stattdessen auf GETTING_STARTED verlinken. |

### Genauigkeit vs. Code

| Schweregrad | Fund |
|---|---|
| Kritisch | `JWT_PRIVATE_KEY_PATH`/`JWT_PUBLIC_KEY_PATH` (dokumentiert in `.env.example:21-22`, `docs/CONFIGURATION_AND_ENV.md:20-21`) werden vom Code nicht gelesen — `cmd/server/main.go:130` liest `JWT_SECRET_KEY_PATH`; `docker-compose.yml` hardcodet den Pfad zusätzlich direkt. |
| Kritisch | `STORAGE_LOCAL_DIR` (`.env.example:25`) wird nirgends im Code gelesen — tatsächlich verwendet wird `STORAGE_PATH` (`cmd/server/main.go:126`; `docker-compose.yml:30,59`; `internal/launcher/templates.go:39,68`). |
| Kritisch | Rollen-Enum-Mismatch (siehe Executive Summary #1). |
| Moderat | KI-Modellname-Inkonsistenz: `docs/AI_GATEWAY.md:43,49` nennt `gemma2:12b`, alle anderen Stellen (README.md:162,185; CONFIGURATION_AND_ENV.md:19,60; FAQ_AND_TROUBLESHOOTING.md; `.env.example:33`; `docker-compose.yml:37`) nutzen konsistent `gemma4:12b`. |
| Moderat | Migrationsanzahl falsch: README-Diagramm (`README.md:42`) sagt "00001-00008", `docs/DEPLOYMENT.md:38` sagt "00001 bis 00007" — real liegen 10 Migrationsdateien (`00001`-`00010`) vor. Beide Angaben sind falsch und widersprechen sich zusätzlich gegenseitig. |
| Moderat | Go-Versionsangabe an 3 Stellen falsch (siehe Executive Summary #6). |
| Moderat | `docs/CONFIGURATION_AND_ENV.md`s Env-Var-Tabelle lässt tatsächlich benötigte Variablen aus (`DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` — Pflicht laut `docker-compose.yml:75-77`; `DOMAIN`, `INITIAL_ADMIN_EMAIL`, `AI_API_KEY`, `AI_BASE_URL`). Umgekehrt fehlen in `.env.example` die dokumentierten `DEMO_MODE` und `INITIAL_ADMIN_PASSWORD`. Doc-Tabelle, `.env.example` und `docker-compose.yml` sind drei nicht deckungsgleiche Quellen. |
| Minor | `STORAGE_PATH`-Default-Wert widersprüchlich: Doku sagt `/data/storage`, Code-Default außerhalb Docker ist `./data/storage` (relativ) — `cmd/server/main.go:127`. |
| Minor | `docs/DEPLOYMENT.md:49-67` referenziert `docker-compose.prod.yml`, das im Repo nicht existiert (nur `docker-compose.yml` und `docker-compose.demo.yml`). |
| Minor | Defekter Badge-Anchor: `README.md:10` verlinkt `#-ressourceneffizienz--low-resource-budget`, tatsächliche Überschrift (`README.md:123`) erzeugt diesen Slug nicht — Link dürfte auf GitHub ins Leere laufen. |
| Minor | Beispiel-Connector-Token in `API.md`/`API_REFERENCE.md`/`D2D_AND_CONNECTORS.md` (`openlocalcrm-secret-connector-token`) passt nicht zum tatsächlichen Demo-Default (`demo-connector-token`, `internal/server/router.go:181`) — Copy-Paste der Doku-Beispiele scheitert mit 401. |
| Minor | `API_REFERENCE.md:176-177` zeigt fiktives Kurz-ID-Format (`"contact_id": "c-1724541234"`), tatsächlicher Handler (`internal/server/handlers/connectors.go:51`) liefert die reale Record-ID. |

### Vollständigkeit

| Schweregrad | Fund |
|---|---|
| Moderat | Kein `CONTRIBUTING.md`, keine Contribution-Workflow-Sektion in README/Docs. |
| Moderat | Keine dokumentierten Prerequisites für native (Nicht-Docker-)Entwicklung — README "Option 4" und GETTING_STARTED.md nennen keine Go-/Node-Version für `make build`. |
| Minor | Reale, registrierte Routen ohne Doku-Abdeckung: `companies`, `todos`, `notifications`, CSV Export/Import, Settings Export/Import, `calculator`, KI-Knowledgebase, `research/*`, `parse-bill` (Abgleich gegen `internal/server/router.go`) — obwohl `API_REFERENCE.md` "vollständige Dokumentation aller REST-Endpunkte" beansprucht. |

### Lesbarkeit

| Schweregrad | Fund |
|---|---|
| Minor | Kein Mixed-Language-/Boilerplate-Problem gefunden — alle Docs konsistent auf Deutsch, keine TODO/Lorem-Ipsum-Reste. |
| Minor | README mischt Marketing-Ton (emoji-lastige Feature-Bullets) mit technischer Referenz im selben Dokument; der Windows-Installer-Walkthrough (~100 Zeilen) ist überproportional groß gegenüber den anderen drei Quickstart-Optionen zusammen — Kandidat für Auslagerung in eigenes Dokument. |
| Hinweis | Gesamte Doku ist ausschließlich auf Deutsch (kein englisches Pendant) — für den Zielmarkt (deutsche Vertriebsteams) plausibel, schränkt aber die internationale OSS-Reichweite/Contributor-Basis ein. |

---

## Kategorie C — Code-Implementierung

| Schweregrad | Fund |
|---|---|
| Kritisch | Rollen-Enum-Mismatch, s. o. — betrifft auch Laufzeitverhalten, nicht nur Doku. |
| Moderat | `web/dist/*` committed (s. Kategorie A) — Korrektheitsrisiko, falls Embed auf diesen Stand statt frischem Build zugreift. |
| Moderat | Verworfene Audit-Log-Fehler: `_ = s.audit.Log(...)` in `internal/core/todo/service.go:87,104,182,194` und `internal/core/company/service.go` (7 Stellen). Scheitert die Audit-Persistierung, läuft die eigentliche CRUD-Operation trotzdem still durch — für ein CRM mit Compliance-/Audit-Anspruch relevant. |
| Minor | `web/package.json` deklariert sowohl `react-router` (`^8.3.1`) als auch `react-router-dom` (`^7.18.2`); im Code wird ausschließlich `react-router-dom` importiert (10 Dateien) — `react-router` ist ungenutzt. |
| Minor | SSE-Stream-Endpunkt (`/events/stream`, `internal/server/router.go` ~L105) implementiert Bearer-/Cookie-Token-Prüfung manuell außerhalb der Standard-`AuthMiddleware`-Gruppe — zweiter, redundanter Auth-Check-Pfad, der über Zeit divergieren kann. |
| Minor | Keine Frontend-Unit-/Component-Tests (kein vitest/jest, keine `*.test.tsx`) — Frontend-Testabdeckung läuft ausschließlich über 17 Playwright-E2E-Specs. Für eine CRUD-lastige UI vertretbar, aber z. B. Kalkulator-Logik hat keinen isolierten Unit-Test. |
| Info (kein Fund) | Keine SQL-String-Konkatenation gefunden (durchgängig sqlc/pgx parametrisiert). Kein `InsecureSkipVerify`/permissive TLS-Config. Keine übermäßig offene CORS-Konfiguration (kein CORS-Handling überhaupt nötig, da SPA same-origin embedded). Auth-Middleware sauber geschichtet (public vs. protected vs. `RequireRole("ADMIN")`), inkl. Rate-Limiting auf Auth-/TOTP-/AI-Endpunkten. |
| Info (kein Fund) | Migrationen (10 Dateien, `00001`-`00010`) korrekt strukturiert, destruktive Statements ausschließlich in `-- +goose Down`-Abschnitten. |
| Info (kein Fund) | 64 echte Go-Testdateien (~6.584 Zeilen) mit inhaltlichen Assertions, kein Skeleton-Testing. `go build ./...` und `go vet ./...` laufen sauber durch. Keine echten TODO/FIXME/HACK-Marker oder "not implemented"-Kommentare im Code gefunden. |

---

## Priorisierte Fund-Übersicht (nach Schweregrad, nicht chronologisch)

| Datei | Zeile(n) | Kategorie | Schweregrad | Fix-Vorschlag |
|---|---|---|---|---|
| `internal/db/migrations/00001_initial_schema.sql` | 9 | Code/Doku | Kritisch | Rollen-Modell klären: Enum um `VERTRIEB`/`BACKOFFICE` erweitern (neue Migration) oder Doku/Code auf `ADMIN`/`BENUTZER` reduzieren. |
| `.env.example` / `docs/CONFIGURATION_AND_ENV.md` | 20-22 / 20-21 | Doku | Kritisch | `JWT_PRIVATE_KEY_PATH`/`JWT_PUBLIC_KEY_PATH` durch tatsächlich genutztes `JWT_SECRET_KEY_PATH` ersetzen. |
| `.env.example` | 25 | Doku | Kritisch | `STORAGE_LOCAL_DIR` durch `STORAGE_PATH` ersetzen. |
| `docs/AUDIT_REPORT_2026-09-21.html` | — | Repo-Hygiene | Kritisch | Entscheidung treffen: entfernen, oder Credentials/Sensitive-Daten daraus schwärzen (aktuell nur dokumentiert, keine Aktion). |
| `bin/*.exe` | — | Repo-Hygiene | Kritisch | Aus Git-Tracking lösen, künftig als GitHub-Release-Artefakt ausliefern (aktuell nur dokumentiert, keine Aktion). |
| `docs/API.md` | ganze Datei | Doku | Kritisch | Mit `docs/API_REFERENCE.md` zusammenführen oder entfernen. |
| `README.md` / `docs/ARCHITECTURE.md` | 3,128-129 / 23-24 | Doku | Moderat | Go-Version auf `1.26` korrigieren (gemäß `go.mod:3`). |
| `README.md` | 42 / `docs/DEPLOYMENT.md:38` | Doku | Moderat | Migrationsanzahl auf tatsächliche 10 (`00001`-`00010`) korrigieren. |
| `docs/AI_GATEWAY.md` | 43,49 | Doku | Moderat | `gemma2:12b` → `gemma4:12b`. |
| `docs/FAQ_AND_TROUBLESHOOTING.md` | 30-33 | Doku | Moderat | Rollenanzahl-Aussage an tatsächliches Modell anpassen (abhängig von Klärung oben). |
| `internal/core/todo/service.go`, `internal/core/company/service.go` | mehrere | Code | Moderat | Audit-Log-Fehler mindestens loggen statt verwerfen. |
| `web/dist/*` | — | Code/Repo-Hygiene | Moderat | Aus Git-Tracking entfernen, `.gitignore`-Eintrag ergänzen, Build als CI/Release-Schritt. |
| `docs/CONFIGURATION_AND_ENV.md` | Tabelle | Doku | Moderat | Fehlende Vars (`DB_HOST` etc.) ergänzen, Tabelle mit `.env.example`/`docker-compose.yml` abgleichen. |
| — (fehlende Dateien) | — | Repo-Hygiene | Moderat | `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `CHANGELOG.md` ergänzen. |
| `README.md` | 135 ff. | Doku | Moderat | Quickstart/Demo-Option näher an den Anfang ziehen, Windows-Installer-Walkthrough auslagern. |
| `web/package.json` | — | Code | Minor | Ungenutztes `react-router` entfernen. |
| `.github/.DS_Store` | — | Repo-Hygiene | Minor | Aus Git entfernen, global `.DS_Store` ignorieren (bereits in `.gitignore`, aber diese Datei war schon getrackt vor Ignore-Regel). |
| `README.md` | 10 | Doku | Minor | Badge-Anchor-Link reparieren. |
| `README.md` | 306-338 | Doku | Minor | Statischen Testlauf-Text entfernen, nur CI-Badge behalten. |
| `docs/API_REFERENCE.md` | 276,310,335,351 | Doku | Minor | Abschnittsnummerierung fortlaufend korrigieren. |
