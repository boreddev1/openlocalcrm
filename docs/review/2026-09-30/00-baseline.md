# 00 — Baseline (2026-09-30, HEAD c6bac3e)

## Umgebung
- Go 1.27.1 darwin/arm64 (go.mod fordert 1.26+), Arbeitsbaum sauber.
- Tools: `golangci-lint`, `sqlc` vorhanden; `govulncheck`, `gosec` per `go install` nachinstalliert.

## Gates
| Gate | Ergebnis |
| --- | --- |
| `go vet ./...` | sauber |
| `gofmt -s -l .` | keine Abweichungen |
| `go test -race -coverprofile ./...` | alle Pakete `ok`, kein FAIL, keine Races |
| Gesamt-Coverage | **59,6 %** (Gate ≥ 50 %: erfüllt) |
| Web-Gates (lint/typecheck/format/build) | siehe `07-frontend.md` |
| `golangci-lint`, `govulncheck`, `gosec` | siehe `03-security-auth.md` |

## Coverage pro Paket
| Paket | % |
| --- | --- |
| cmd/server | 2,0 |
| cmd/setup-launcher | 22,6 |
| internal/ai | 74,7 |
| internal/auth | 77,9 |
| internal/connectors | 72,7 |
| core/appointment | 59,5 |
| core/audit | 86,5 |
| core/automation | 78,0 |
| core/backup | 84,8 |
| core/company | 77,4 |
| core/contact | 68,0 |
| core/deal | 58,4 |
| core/email | 57,4 |
| core/emailtemplate | 92,0 |
| core/export | 92,3 |
| core/invite | 86,4 |
| core/note | 54,9 |
| core/notification | 89,7 |
| core/reports | 91,8 |
| core/telephony | 84,2 |
| core/todo | 51,3 |
| internal/crypto | 62,6 |
| internal/db | 10,4 |
| internal/db/demo | 58,2 |
| internal/geocoding | 100 |
| internal/launcher | 61,5 |
| internal/launcher/ui | 0,0 |
| internal/mailclient | 79,6 |
| internal/queue | 58,1 |
| internal/server | 94,2 |
| internal/server/handlers | 70,0 |
| internal/sse | 86,5 |
| internal/storage | 83,0 |
| cmd/worker | keine Testdatei (nicht im Lauf gelistet) |

## Beobachtungen (Detail in 02-coverage.md)
- Schwächste Pakete: `cmd/server` 2 %, `internal/db` 10,4 % (Migrator), `internal/launcher/ui` 0 %, `cmd/setup-launcher` 22,6 %, `cmd/worker` ohne Tests.
- Nahe am Gate: `core/todo` 51,3 %, `core/note` 54,9 %.
- Laufzeit lang: `auth` 112 s, `invite` 111 s, `handlers` 141 s (vermutlich Argon2 in Tests); relevant für Pre-Push-Hook.
- Repo-Root enthält `coverage.out`, `coverage/`, `openlocalcrm-setup.log`, Screenshot-PNG (Tracking-Status in `08-ops-supply-chain.md`).
