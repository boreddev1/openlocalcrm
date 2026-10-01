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

## Offene Punkte
- `web/dist` ist in Git getrackt (4 Dateien). Der Frontend-Fix steckt im Quellcode, nicht im eingecheckten Bundle; `make build` und die Docker-Images bauen das Frontend neu, ein reines `go build` bettet das alte Bundle ein.
- `golangci-lint` läuft mit Go 1.27.1 weiterhin nicht (Toolchain); als Ersatz `go vet` und `gofmt`.
- Nicht Teil von A1: CSRF-Secret wird bei jedem Serverstart neu erzeugt (alle Sitzungen danach 403) → A2.
