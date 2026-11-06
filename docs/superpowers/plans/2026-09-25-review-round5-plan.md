# Plan: OpenLocalCRM In-Depth Source Review — Round 5

## Context

OpenLocalCRM hat bereits 4 Review-Runden hinter sich (alle 2026-09-21, in `docs/`):
`REPO_REVIEW_2026-09-21.md` (Runde 1), `REPO_REVIEW_2026-09-21_round2.md` (Runde 2),
`SECURITY_REVIEW_2026-09-21_round3.md` (Runde 3), `REVIEW_2026-09-21_round4.md` (Runde 4).

Jede Runde fand dasselbe Meta-Muster: Commit-Messages/UI behaupten ein Feature oder einen
Fix sei fertig, tatsächlich war Code Stub, No-Op oder halb verdrahtet (fake SMTP/IMAP,
fake pgvector-Embeddings, simulierter Kalender-Push, No-Op Automation-Trigger, IDOR bei
Deals, Klartext-Secrets im Settings-Export, UWG-Consent/§355-BGB-Widerruf nie persistiert).

Der "SP6"-Commit-Streak (HEAD `c8c7d3c`, zuletzt `55ae299 fix: enforce SP6 data truth and
compliance invariants`) behauptet, alle Runde-4-Findings geschlossen zu haben (verifiziert:
alle zitierten Commits `726a867`, `3e1114b`, `1deee39`, `b8a5621`, `827e08d`, `654d1bf`,
`ed7f7be`, `97407d0`, `55ae299` existieren real in der Historie und Beschreibung passt).

**Ziel dieser Runde 5:** ein skeptischer, evidenzbasierter Review, der nicht wieder auf
Commit-Messages vertraut, sondern jede "fixed"-Behauptung gegen echten Code/Tests/Laufzeit
verifiziert — plus vollständige Abdeckung von Feature-Vollständigkeit, Testabdeckung,
Sicherheit und Code-Qualität über das gesamte Repo (nicht nur den SP6-Diff).

Deliverable: `docs/REVIEW_2026-09-25_round5.md`, reiner Lese-Audit — kein Code wird
in dieser Runde geändert.

## Grundprinzip: skeptisch by default

Für jeden aus einer vorigen Runde übernommenen Finding: Status `CONFIRMED FIXED` /
`PARTIALLY FIXED` / `STILL OPEN` / `NEW`, nie "laut Commit-Message erledigt" — immer
mit file:line-Beleg oder Testlauf. Kommentare/Docstrings/Variablennamen sind kein Beweis;
es zählt der tatsächliche I/O-Endpunkt (Netzwerk-Call, DB-Write) oder ein Test, der bei
Entfernen des Verhaltens fehlschlagen würde.

## Umfang (4 Dimensionen)

1. **Feature-Vollständigkeit / Anti-Fake-Sweep** — sind die Runde-4-Fixes real?
2. **Testabdeckung** — quantitativ (`scripts/coverage.sh`) und qualitativ (prüfen Tests echtes Verhalten oder nur "kein Fehler")?
3. **Sicherheit** — AuthZ-Konsistenz über alle 19 Handler + router.go, Connector-Auth, PII-Guard-Grenzen, Secrets, CSRF/Rate-Limiting.
4. **Code-Qualität** — Error-Handling ("Fehler schlucken, Erfolg melden" kam 2x vor), toter Code/Stubs, Struktur-Konsistenz, AGENTS.md-Invarianten.

Out of scope: Performance/Load-Tests, Infra-Hardening über bereits Bekanntes hinaus, Frontend-Design-Review.

## Pre-Flight (einmalig, vor den Passes)

```bash
git log --oneline -20
make check
scripts/coverage.sh --min 50.0 --diff 50.0
git diff --stat 55ae299~1..HEAD
diff -r internal/db/migrations migrations   # Symlink-Invariante
```

Schlägt `make check` oder Coverage-Gate auf sauberem `main` fehl: das ist selbst ein
CRITICAL-Finding und führt den Report an, statt vergraben zu werden.

## Ausführung: 4 parallele Passes + 1 Synthese-Pass

Jeder Pass unabhängig, keine gemeinsamen Zwischenstände — als parallele Subagenten in
einem Batch dispatchen. Jeder bekommt: diesen Plan, seine Dateiliste unten,
`docs/REVIEW_2026-09-21_round4.md` als Referenz für Prior Findings, Anweisung für
file:line-belegte Findings im Format aus Abschnitt "Report-Format".

### Pass A — Feature-Truth-Verifikation

Tabelle: jedes Runde-4-Finding gegen seinen behaupteten Fix-Commit, mit exaktem Check:

| Prior Finding | Fix-Commit | Verifikation |
|---|---|---|
| Kein SMTP/IMAP, Email täuscht Erfolg vor | `726a867` | `internal/core/email/service.go` (464 Zeilen) ganz lesen: echter `net/smtp`/IMAP-Client-Call bei `SendEmail`/`SyncAccount`/`TestAccountConnection`? `go.mod` auf IMAP-Dependency prüfen, tatsächlich genutzt? Tests in `service_test.go` (439 Zeilen): welche prüfen echten Netzwerkpfad vs. nur DB-Roundtrip? |
| Fake pgvector, KB-Suche ohne echtes Embedding | `3e1114b` | `00014_pgvector_768.sql` vs `00015_resize_embedding_vector.sql` — Dimension-Wechsel mit tatsächlicher Embedding-Dimension in `internal/ai/*` abgleichen (Commit behauptet 1024-dim). Embedding-Erzeugung: echter Modell-Call oder Stub-Vektor? KB-Suche nutzt pgvector-Operator (`<=>`/`<->`) in `internal/db/queries/*.sql`, nicht LIKE-Fallback? |
| Kalender-Push zu MS/Google versprochen, nie gebaut | `b8a5621` | Frontend (`web/src` grep "Google Calendar\|Microsoft\|Outlook") frei von Fake-UI? `internal/core/appointment/service.go` frei von Rest-Fake-Pfad? ICS-Download (router.go ~445) erzwingt Ownership-Check? |
| Fake Call-Synthese Telephony | `827e08d` | `internal/core/telephony/service.go` (89 Zeilen, ganz lesen): `LogCall` nur echte Metadaten, keine erfundenen Outcomes/Dauer/Transkripte? **Cross-Ref Pass B:** `router.go` ~427 `/telephony/calls` ohne `RequireAnyRole(...)` — Absicht oder Lücke? |
| Automation-Trigger tot, Approve-Step nie aufgerufen | `1deee39` | `internal/core/automation/service.go` (716 Zeilen, größte Core-Datei): Trigger feuern bei echten Events (Kontakt/Deal-Create, Inbound-Mail)? `web/src` grep auf `POST /automations/runs/{id}/approve` — UI ruft es wirklich? |
| UWG-Consent/Energiedaten in UI erfasst, nie persistiert | `ed7f7be`, `654d1bf`, `2d53563` | Ein Consent-Feld end-to-end verfolgen: Web-Formularfeld → Handler-Request-Struct → `internal/core/contact/service.go` → SQL-Query → Migration, die Spalte hinzufügt. Roundtrip: `Get`/`List` liefert Feld zurück, `export.go` inkludiert es in CSV. Test schreibt consent=true, liest zurück, prüft maskierten Audit-Diff-Eintrag? |
| §355-BGB-Widerruf strukturell unerreichbar | `97407d0`, `55ae299` | `internal/core/deal/service.go` `Revoke`: direktes `PUT /deals/{id}` mit stage="Widerrufen" abgelehnt? `POST /deals/{id}/revoke` (router.go ~366, Rollen ohne BENUTZER — Absicht?) macht echten Übergang + Audit-Log? Web-UI ruft `/revoke`, nicht generisches Update? |
| Silent-Fallback im LLM-Gateway; Note-Synthese erstellt Deals aus erfundener "Kaufabsicht" | Kein SP6-Commit behauptet Fix | `internal/ai/gateway.go` auf Hardcoded-Fallback bei Provider-Ausfall prüfen. `noteH.Synthesize`/`internal/core/note/service.go` `Synthesize`: erstellt automatisch Deals ohne Human-in-the-Loop? **Als STILL OPEN behandeln, bis bewiesen.** |

Zusätzlicher frischer Sweep: `web/src` grep auf "Bald verfügbar"/TODO/FIXME/Stub-Strings
gegen Backend-Realität; `internal/connectors/connector.go` Auth-Modell (statischer Bearer-Token,
`subtle.ConstantTimeCompare`, kein per-Connector-Scoping/HMAC/Replay-Schutz) als Finding
formalisieren; `internal/ai/guard.go` an jedem Outbound-LLM-Call-Site (`Chat`, `TriageEmail`,
`ResearchCompany`, `ParseBill`, `Synthesize`) tatsächlich aktiv?; `aiH.ParseBill` (Runde 2:
fake Endpoint) aktueller Stand?

### Pass B — Sicherheit & Autorisierung

**Dateien:** `internal/server/router.go` (496 Zeilen, komplett), alle 19 Dateien in
`internal/server/handlers/`, `internal/auth/*.go`, `internal/connectors/connector.go`,
`internal/storage/*.go`, `internal/ai/guard.go`.

**Methode:** eine Master-Tabelle über jede registrierte Route (Methode+Pfad, Handler,
Auth-Middleware, Rollen) UND — entscheidend — ob Handler/Service-Schicht Record-Level-
Ownership durchsetzt (die Runde-3-Deal-IDOR war ein Rollen-Check-Pass, aber
Ownership-Check-Fail: jeder VERTRIEB-User konnte jeden Deal sehen/editieren, nicht nur
eigene). Bei contacts/companies/deals/todos/notes/appointments: `Get`/`Update`/`Delete`
bis in `internal/db/queries/<domain>.sql` verfolgen — filtert SQL nach Owner, oder ist
das Modell "alle internen Rollen sehen alles" explizit dokumentiert?

Konkret abzuschließende Punkte:
1. **Settings-Export-Secrets-Leak (Runde 4 CRITICAL):** `settings.go` `ExportSettings` — Klartext-Admin-Passwort/AI-Key/Webhook-Token wirklich raus aus Export-Payload?
2. **CSRF-Bypass via Header-Prefix (Runde 4):** `internal/server/csrf.go`/`internal/auth/csrf.go` validiert echten Token, nicht nur Header-Präsenz?
3. **Rate-Limit-Bypass via spoofbaren Headern:** `middleware.RealIP` entfernt? `ClientIP`-Implementierung vertraut keinen Client-Headern ohne Proxy-Allowlist? Login-Rate-Limit sinnvoll konfiguriert?
4. **Connector-Auth-Modell:** akzeptiertes Risiko für Single-Tenant oder Finding? Check `docs/D2D_AND_CONNECTORS.md`.
5. **Rollen-Modell-Sanity:** 4-Rollen-Enum (ADMIN/VERTRIEB/BACKOFFICE/BENUTZER) vollständig? Telephony/Notifications/AI/Calculator ohne Rollen-Middleware = stilistische Inkonsistenz, kein echtes Loch — explizit so benennen. Echte Einschränkungen (reports nur ADMIN+BACKOFFICE, automations/export nur ADMIN, deal.Revoke ohne BENUTZER) gegen dokumentierte Absicht prüfen.
6. **Storage Path Traversal:** `sanitizeRelPath` deckt auch Windows-Separatoren/Symlink-Escapes ab?
7. **AI-PII-Guard-Lücke:** regex-only (kein NER/Freitext-Namen/Adressen) formal dokumentieren, jeden Outbound-Call-Site auflisten.
8. **gosec konfiguriert, läuft aber nie in CI:** `.golangci.yml` listet gosec, aber `ci.yml`/`docker-publish.yml` haben keinen gosec/CodeQL/Trivy-Step — prüfen ob `make lint`/CI überhaupt `golangci-lint` aufruft (AGENTS.md: `make lint` = `go vet` + Web-Lint — golangci-lint evtl. komplett unverdrahtet).

### Pass C — Testabdeckung

1. `scripts/coverage.sh` real laufen lassen, Pro-Paket-Zahlen erfassen, auch wenn Gesamt-Gate grün ist (kann dünne Pakete verdecken).
2. LOC-Verhältnis Service/Test als Heuristik: `appointment` (415/116), `todo` (206/74), `note` (277/74) auffällig klein — priorisieren.
3. Qualitativ pro Test-Datei: `assert.NoError` allein ohne Wertprüfung = Shallow-Signal, vs. Tests die konkrete Werte/DB-Zustand/Fehlertyp prüfen. Verdikt pro Paket: shallow/adequate/thorough.
4. Speziell: wurden zu SP6-Features (Consent/Revocation/Email/Embedding) Tests hinzugefügt, die das echte Verhalten aus Pass A prüfen (nicht nur DB-Roundtrip)?
5. `e2e/*.spec.ts` (~20) auf Abdeckung neuer Flows (Email-Accounts, Consent/Revocation-UI, KB-Suche) prüfen.
6. `scripts/diff-coverage.go` kurz lesen — zählt Base-Ref korrekt?

### Pass D — Code-Qualität & Invarianten

1. **Error-Swallowing-Sweep:** grep `_ = err`, Log-und-weiter statt Propagate, Handler die Erfolg meldet bevor alle Teiloperationen bestätigt sind. `backup.go` (Runde-2-Bug) und `export/service.go` gezielt prüfen.
2. **Toter Code/Stubs:** grep TODO/FIXME/"not implemented"/stub/mock (ohne Testdateien), cross-linken mit Pass A.
3. **Struktur-Konsistenz:** 13 Core-Domains vs. 19 Handler (6 ohne Core-Gegenstück: ai/auth/backup/calculator/connectors/settings/users — Absicht? Testabdeckung trotz Umgehung des Core-Musters, besonders sicherheitsrelevant bei auth/settings/backup). `web/src/components/`-Unterordner nur für ai/calculator/common/contacts/notes/telephony/layout — Rest page-local: Absicht oder Drift?
4. **AGENTS.md-Invarianten:** Symlink (aus Pre-Flight), sqlc-Staleness (`sqlc generate` in Scratch-Kopie, Diff gegen committed Files), `make check-licenses` — neue SP6-Dependencies (IMAP/Embedding-Client) permissiv lizenziert?, keine neue ORM-artige Dependency.

## Selbstverifikations-Protokoll (vor jedem Report-Eintrag)

1. Zitierte Datei/Zeile live erneut öffnen, nicht aus Gedächtnis übernehmen.
2. STILL OPEN/NEW-Behauptung: wenn machbar mit Testlauf/curl gegen `make run-local`/vollständiger Call-Graph-Trace belegen — "sieht nach Fake aus" ist schwächer als "jeder Caller verfolgt, keiner erreicht echten I/O".
3. CONFIRMED FIXED: nicht schon weil eine Datei im Fix-Commit angefasst wurde — konkretes Verhalten bestätigen.
4. Coverage-Zahlen müssen aus echtem `scripts/coverage.sh`-Lauf stammen, LOC-Vergleich nur als Heuristik wo man hinschaut.
5. Pass-B-Tabelle: Zeilenzahl gegen `grep -c` der Route-Registrierungen in router.go abgleichen — keine Route übersehen.

## Report-Format (analog `docs/REVIEW_2026-09-21_round4.md`)

`docs/REVIEW_2026-09-25_round5.md`:
1. Header: Scope, HEAD-Commit, referenzierte Vorrunden, Methode (4 Passes + Synthese), Status "Read-only audit. No code changed."
2. Executive Summary (5-8 Punkte, zuletzt geschrieben, nach allen Passes).
3. Prior-Findings-Statustabelle (eine Zeile pro Runde 2/3/4-Finding-ID).
4. Neue Findings, gruppiert nach Pass, Severity CRITICAL/HIGH/MEDIUM/LOW, file:line, Impact, Remediation-Richtung (keine Implementierung).
5. Coverage-Anhang (Zahlen + shallow/adequate/thorough-Tabelle).
6. Route/AuthZ-Anhang (Master-Tabelle aus Pass B).
7. Schlussnotiz: ob Follow-up-Remediation-Plan nötig (nicht Teil dieser Runde).

## Sequenzierung

1. Pre-Flight (sequenziell, ~10 Min).
2. Pass A, B, C, D parallel dispatchen (je ein Subagent) — Pass C bekommt Runde-4-Tabelle aus Pass A vorab, um "neue" Features nicht doppelt zu identifizieren.
3. Synthese: alle 4 Rohbefunde lesen, Selbstverifikation auf überraschende/High-Severity-Punkte, Cross-Pass-Duplikate auflösen (z.B. Telephony-Route-Gate erscheint in A und B — einmal in B behalten, Cross-Ref aus A), Executive Summary + finalen Report schreiben.
4. Optional danach (nicht Teil dieses Plans): Entscheidung über Folge-Remediation-Plan.

## Verifikation

- Alle zitierten Commit-Hashes (`726a867`, `3e1114b`, `1deee39`, `b8a5621`, `827e08d`, `654d1bf`, `ed7f7be`, `97407d0`, `55ae299`, `c8c7d3c`) real in `git log` verifiziert.
- Alle referenzierten Vorrunden-Docs (`docs/REPO_REVIEW_2026-09-21.md`, `_round2.md`, `SECURITY_REVIEW_2026-09-21_round3.md`, `REVIEW_2026-09-21_round4.md`) existieren.
- Erfolg dieser Runde: `docs/REVIEW_2026-09-25_round5.md` liegt vor, `make check` + `scripts/coverage.sh` liefen real, jede Zeile der Prior-Findings-Tabelle hat konkreten Beleg statt Vermutung.
