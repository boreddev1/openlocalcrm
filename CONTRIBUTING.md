# Mitwirken an OpenLocalCRM (Contributing Guide)

Vielen Dank für Ihr Interesse an **OpenLocalCRM**! Wir freuen uns über Beiträge aus der Community zur kontinuierlichen Verbesserung von Sicherheit, Performance und Benutzerfreundlichkeit.

---

## 🛠️ Entwicklungsumgebung einrichten

### Voraussetzungen:
- **Go 1.26+**
- **Node.js 22+** und **pnpm 12.4.1** — die `overrides` liegen in `web/pnpm-workspace.yaml`,
  und pnpm 9 liest sie weder dort noch aus den Lockfile-`settings`, die sie dort recorden
  (gemessen: `9.15.9` ❌ `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`, `12.4.1` ✅). Dieselbe Version nutzen `ci.yml`,
  `docker-publish.yml` und `Dockerfile.server`. `packageManager` in `web/package.json` ist bewusst
  **nicht** gesetzt: pnpm 12.4 verweigert mit dem Feld `--frozen-lockfile`, solange der
  `pnpm-lock.yaml` keine pm-dependencies recordet.
- **Docker & Docker Compose** (optional für lokalen Komplett-Stack)

### Repository klonen & Abhängigkeiten installieren:
```bash
git clone https://github.com/openlocalcrm/openlocalcrm.git
cd openlocalcrm

# Pre-Commit & Pre-Push Hooks aktivieren:
make setup-hooks

# Web-Frontend bauen:
cd web && pnpm install && pnpm build && cd ..
```

---

## 🧪 Tests & Qualitätssicherung

Vor dem Einreichen eines Pull Requests müssen alle lokalen Prüfungen erfolgreich durchlaufen:

```bash
# Alle Go-Tests ausführen:
make test

# Frontend Linting & Typecheck:
make lint

# Lizenz-Compliance-Scan:
make check-licenses

# Playwright E2E (baut bin/crm-server, startet Demo-Backend + Vite-Dev-Server):
make test-e2e

# Vollständige Suite (Hooks, Formatierung, Linting, Tests):
make check
```

### E2E-Port, wenn 8080 belegt ist

`make test-e2e` startet das Demo-Backend auf Port **8080** — dem dokumentierten Standard,
der auch in CI gilt. Auf Rechnern, wo 8080 schon vergeben ist (z. B. ein IDE-Server),
lässt sich der Port verschieben:

```bash
E2E_BACKEND_PORT=8090 make test-e2e
```

Die Variable wirkt an **zwei** Stellen gleichzeitig, weil beide zusammengehören:
`playwright.config.ts` (Port des Demo-Backends) und `web/vite.config.ts` (Ziel des
`/api`- und `/events`-Proxys). Nur den einen zu ändern, lässt die SPA gegen einen Port
proxyen, an dem nichts lauscht — jede API-Spec schlägt dann fehl, ohne dass die App
defekt ist. Ohne die Variable bleibt es bei 8080.

---

## 🌿 Branch- & Commit-Konventionen

- Nutzen Sie aussagekräftige Feature-Branches (z. B. `feat/email-threading`, `fix/csrf-rotation`).
- Commits folgen dem [Conventional Commits](https://www.conventionalcommits.org/)-Schema:
  - `feat: ...` für neue Funktionen
  - `fix: ...` für Fehlerbehebungen
  - `docs: ...` für Dokumentationsänderungen
  - `test: ...` für zusätzliche Tests
  - `refactor: ...` für Code-Refactoring ohne Verhaltensänderung

---

## 🚀 Pull Request Workflow

1. Erstellen Sie einen Fork des Repositories.
2. Führen Sie Ihre Änderungen auf einem dedizierten Branch durch.
3. Ergänzen Sie Tests für neue Logik oder behobene Fehler.
4. Prüfen Sie, dass `make check` zu 100 % grün durchläuft.
5. Reichen Sie Ihren PR gegen den `main`-Branch ein und beschreiben Sie den Hintergrund und die Änderungen im PR-Template.
