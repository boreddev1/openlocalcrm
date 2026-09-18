## Zusammenfassung der Änderungen
<!-- Kurze Beschreibung, welches Problem behoben oder welches Feature implementiert wurde -->

## Art der Änderung
- [ ] 🐛 Bugfix (Fehlerbehebung ohne Breaking Change)
- [ ] ✨ Neues Feature (Rückwärtskompatible Erweiterung)
- [ ] 💥 Breaking Change (Änderung, die bestehende Schnittstellen oder Schemas bricht)
- [ ] ♻️ Refactoring / Code-Optimierung
- [ ] 📝 Dokumentations-Update
- [ ] 🔧 Tooling / CI/CD / Dependencies

## Qualitäts- & Sicherheits-Checkliste
- [ ] Meine Änderungen entsprechen den Projekt-Coding-Standards (`make check` läuft lokal 100% grün durch)
- [ ] Go-Code ist mit `gofmt` formatiert (`make fmt`)
- [ ] Frontend-Code besteht TypeScript-Typecheck und ESLint (`make lint`)
- [ ] Alle Unit- und Integrationstests laufen erfolgreich (`make test`)
- [ ] Bei Datenbank-Änderungen:
  - [ ] Neue Migrationsdatei in `migrations/` UND `internal/db/migrations/` angelegt
  - [ ] `sqlc generate` ausgeführt, falls Queries geändert wurden
  - [ ] Dual-Backing in `internal/db/demo/querier.go` für Demo-Modus aktualisiert
- [ ] Bei neuen Dependencies: Keine viralen Lizenzen (GPL/AGPL), 100% konform mit Section 22.2 (`make check-licenses`)
- [ ] Keine Secrets, `.env`-Dateien oder API-Schlüssel gestagt
