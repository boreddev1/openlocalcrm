# Mitwirken an OpenLocalCRM (Contributing Guide)

Vielen Dank für Ihr Interesse an **OpenLocalCRM**! Wir freuen uns über Beiträge aus der Community zur kontinuierlichen Verbesserung von Sicherheit, Performance und Benutzerfreundlichkeit.

---

## 🛠️ Entwicklungsumgebung einrichten

### Voraussetzungen:
- **Go 1.26+**
- **Node.js 22+** und **pnpm 9+**
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

# Vollständige Suite (Hooks, Formatierung, Linting, Tests):
make check
```

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
