# 09 — A1 Verifikation (2026-10-01, Branch fix/a1-normal-operation)

Commits: `54d9486` (R-01), `9a0840d` (N-01), `e5243e7` (R-04), `c91dfa1` (R-08 Backend), `3b31775` (R-08 Frontend).

## Live-Regression
Frisch gebauter `crm-server` (`DEMO_MODE=false`) gegen eine **leere** `pgvector/pgvector:pg16`-Datenbank. Migration `00010` wurde **nicht** manuell als angewendet markiert.

| Befund | Prüfung | Ergebnis |
| --- | --- | --- |
| R-01 | Server startet auf leerer DB | 21 Migrationen angewendet, `listening on :18082` (vorher Abbruch bei 00010) |
| N-01 | `POST /deals` ohne `value` | 201, `value = 0.00`, Owner = Ersteller (vorher 500) |
| N-01 | `value` als String `"5000.50"` / Zahl `7500` | beide 201, gespeichert als `5000.50` / `7500.00` |
| N-01 | `value: "abc"` | 400 |
| R-04 | Deal mit Kontakt verknüpft, `PUT` ohne Links, `value` als Zahl `6000` | 200, `contact_id` und `assigned_to` bleiben gesetzt, `value = 6000.00` (vorher beide NULL) |
| R-08 | `POST /auth/refresh` mit gültigem CSRF-Paar | 200 (vorher 403) |
| R-08 | `POST /auth/refresh` ohne CSRF-Header | 403 |
| R-08 | `POST /contacts` mit rotiertem CSRF-Token nach Refresh | 201 |
| R-08 | `POST /auth/logout` | 200 |
| R-08 | Refresh-Token nach Logout erneut verwenden | 401 (Token wirklich widerrufen) |

## Tests
- Neu und vorher rot (je mit Fix grün): `TestMigrationsReplayOnEmptyDatabase`, `TestDealHandler_Create_*`, `TestDealHandler_Update_AcceptsNumericValue`, `TestDealCreateAgainstRealPostgres` (ohne Fix: `null value in column "value"`, 500), `TestDealService_UpdateKeepsLinksAndOwnerWhenOmitted`, `TestTodoService_UpdateKeepsLinksWhenOmitted`, `TestCSRFDoubleSubmit_*`, `TestCSRF_RefreshAndLogoutWorkWithoutAccessToken`, E2E `auth.spec.ts` (Retry mit rotiertem Token).
- `go test ./internal/core/... ./internal/auth ./internal/server/...` grün; Web: `pnpm format:check`, `lint` (0 Fehler, 2 Warnungen), `typecheck`, `build` sauber; Playwright `auth.spec` 14 bestanden.

## Nachträge (alle offenen Punkte aus A1 geschlossen)

| Punkt | Commit | Ergebnis |
| --- | --- | --- |
| Deal-Wert: NaN/Infinity/negativ/zu groß, `""` bei Update, Präzision | `1f38562` | Review-Fund; live: 400 für ungültige Werte, Max-Wert 201, Sales-Report 200 |
| R-06 Backup: `prev_hash`/`hash` in Export, Restore, Drill | `a357d8f` | Drill und Restore des eigenen Exports funktionieren; alte Backups ohne die Spalten laden weiter |
| R-06 Backup: Restore scheiterte bei 2+ Zeilen pro Tabelle (`invalid character ','`) | `afa063c` | Live-Roundtrip: Export, Drill, Merge, Replace; Zeilenzahlen identisch (außer `audit_logs`, das eigene Events schreibt) |
| `Request`-Objekte beim CSRF-Retry | `6f7e22d` | E2E-Test rot ohne, grün mit Fix |
| `web/dist` neu gebaut | `c71184a` | Eingebettetes Bundle entspricht dem Quellcode |
| `golangci-lint` v2: Config, 0 Befunde, `make lint` und CI | `3ffce2f`…`55c57c1` | 9 Commits; 56 begründete `//nolint`; Hardening: Zip-Eintrag max. 1 GiB, MaxBytesReader bei Backup-Upload und CSV-Import, int32-Sättigung, Backup-Dateien 0600 |
| Refresh-Rotation gab bei Signaturfehler `err` statt `txErr` zurück (Sicherheitshinweis) | `df06d79` (beiläufig), Test `1a36df7` | Regressionstest erzwingt Signaturfehler und prüft Rollback |

Gate: `golangci-lint run ./...` 0 Befunde, `go vet`, `gofmt`, `go test -race ./...`, `scripts/test-backup-integration.sh` (Backup, Invite, Ownership, Session gegen echte Postgres) grün.

## Offene Punkte
- Postgres-Tests im Backup-Paket erwarten eine saubere DB (Replace-Modus); `scripts/test-backup-integration.sh` stellt sie bereit.
- CI-Action `golangci-lint-action@v9` nicht per SHA gepinnt (separater Task).
- Nicht Teil von A1: CSRF-Secret wird bei jedem Serverstart neu erzeugt (alle Sitzungen danach 403) → A2.
