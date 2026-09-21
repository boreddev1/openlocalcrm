# 🏢 OpenLocalCRM (Single-Tenant Edition v3)

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![React Version](https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react)](https://react.dev/)
[![Vite](https://img.shields.io/badge/Vite-6.4-646CFF?style=flat&logo=vite)](https://vitejs.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16%20%2B%20pgvector-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Playwright E2E](https://img.shields.io/badge/Playwright%20E2E-27%20passed%20(100%25)-2EAD33?style=flat&logo=playwright)](https://playwright.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-emerald.svg)](LICENSE)
[![RAM Budget](https://img.shields.io/badge/RAM%20Budget-%3C%20420%20MB-blue.svg)](#-ressourceneffizienz--420-mb-ram-budget)
[![CI/CD](https://img.shields.io/badge/CI%2FCD-GitHub%20Actions-2088FF?style=flat&logo=githubactions)](https://github.com/boreddev1/openlocalcrm/actions)

**OpenLocalCRM** ist ein hochperformantes, modulares Open-Source-CRM-System nach **Single-Tenant-Architektur**. Entwickelt für Vertriebsteams in Deutschland (insb. Direktvertrieb, Energie, Handwerk und B2B), vereint es maximale Datensouveränität (100 % DSGVO- & UWG-konform), integrierte **Gemma 12B KI-Funktionen mit aktiven PII-Datenschutzfiltern**, reaktive **Workflows mit Human-in-the-Loop (HITL)** und einen extrem ressourcenschonenden Betrieb (< 420 MB RAM).

---

## 📑 Inhaltsverzeichnis
- [🚀 Schnellstart (Demo in 30 Sekunden)](#-schnellstart--deployment)
- [🏛️ Systemarchitektur](#️-systemarchitektur)
- [⚡ Vollständige Kernfunktionen (§1–§22)](#-vollständige-kernfunktionen-122)
- [🛡️ Sicherheitsarchitektur & Enterprise-Hardening](#️-sicherheitsarchitektur--enterprise-hardening)
- [💾 Ressourceneffizienz (< 420 MB RAM Budget)](#-ressourceneffizienz--420-mb-ram-budget)
- [📚 Vollständige Dokumentation & Wiki](#-vollständige-dokumentation--wiki)
- [📄 Lizenz & Mitwirken](#-lizenz)

---

## 🏛️ Systemarchitektur

```mermaid
flowchart TD
    subgraph Reverse_Proxy ["🌐 Reverse Proxy & Edge"]
        Caddy["Caddy Proxy (:80 / :443)<br/>Auto-TLS • Security Headers • SSE Unbuffered • Gzip/Zstd"]
    end

    subgraph App_Server ["🚀 Standalone Single-Tenant Go Server"]
        ChiRouter["Go Server (Chi Router :8080)"]
        EmbeddedSPA["Eingebettetes React 18 SPA (0 MB Extra Server RAM)"]
        CmdK["Globale Schnellsuche (Cmd+K / Ctrl+K Palette)"]
        AuthModule["Argon2id • Ed25519 JWTs (HttpOnly) • TOTP 2FA"]
        SSEHub["SSE Event Stream Hub"]
        StorageSvc["StorageService (SHA-256 Deduplizierung)"]
        AIGateway["KI Gateway (Gemma 12B + PII Prompt Guards)"]
        Automations["Workflow & Automations Engine (HITL)"]
        Connectors["Lead-Intake Webhook Engine"]
    end

    subgraph Worker_Daemon ["⚙️ Background Processing"]
        RiverWorker["Go River Queue Daemon (:8081)<br/>E-Mail IMAP Sync • KI Triage • Audit Logging"]
    end

    subgraph Database ["🗄️ PostgreSQL 16 Storage"]
        PostgresDB[("PostgreSQL 16 Engine<br/>Auto-Migrations 00001-00011 • pgvector • Revisionssicher")]
    end

    Client["💻 Browser / Mobile Client"] -->|"HTTPS / WSS"| Caddy
    Caddy -->|"Reverse Proxy / SSE"| ChiRouter
    ChiRouter --> EmbeddedSPA
    EmbeddedSPA --> CmdK
    ChiRouter --> AuthModule
    ChiRouter --> SSEHub
    ChiRouter --> StorageSvc
    ChiRouter --> AIGateway
    ChiRouter --> Automations
    ChiRouter --> Connectors
    ChiRouter -->|"jackc/pgx/v5 (sqlc)"| PostgresDB
    RiverWorker -->|"Transactional Queue"| PostgresDB
    RiverWorker -.->|"Job Notifications"| SSEHub
```

---

## ⚡ Vollständige Kernfunktionen (§1–§22)

### 1. 📇 CRM Core Data & Vertriebs-Pipeline
- **Kontakte & Leads:** Verwaltung aller Kundendaten mit Anschrift, Geokoordinaten und Zählernummern.
- **UWG § 7 Werbeeinwilligungen:** Getrennte, auditierbare Einwilligungshäkchen für Telefon (`consent_phone`) und E-Mail (`consent_email`).
- **Energiedaten & D2D-Vertikale:** Zählernummern, Jahresverbrauch (Strom/Gas in kWh) und Eigentümerstatus.
- **Deal Kanban Pipeline:** Interaktives Board (`@dnd-kit`) über alle Phasen:
  `Lead / Erstkontakt` ➔ `Qualifiziert` ➔ `Angebot vorliegend` ➔ `Verhandlung` ➔ `Gewonnen` / `Verloren` / `Widerrufen nach § 355 BGB`.
- **Aufgaben & Termine:** Prioritätsstufen (`URGENT`, `HIGH`, `MEDIUM`, `LOW`), Fälligkeiten und Erledigt-Status.
- **Revisionssicheres Audit-Log:** Unveränderbare Historie (`audit_logs`) mit JSONB-Diffs (`old_values`, `new_values`) zu jeder Mutation.

### 2. ⚡ Workflows & Automatisierungs-Engine (§6.4 / §9.4)
- **Automations-Dashboard (`/automations`):** 
  - Standard-Routinen: *Erstkontakt & Qualifizierung (Neuer Lead)*, *Deal-Abschluss Routine (Phase: WON)*, *Inaktivitäts-Reaktivierung (SLA 30 Tage)*.
  - **Human-in-the-Loop (HITL) Freigaben:** Vorbereitete Aktionen verweilen im Status `WAITING_APPROVAL` bis zur expliziten Freigabe.
  - **Custom Routine Builder:** Konfiguration von Triggern, Zielen und mehrstufigen Aktionen.

### 3. 🔍 Globale Schnellsuche & Quick-Switcher (`⌘K` / `Ctrl+K`) (§3.7)
- Durchsucht Kontakte, Firmen, Deals, Termine und E-Mails mit Sofort-Tastaturkürzel `⌘K` von überall im System.

### 4. 🧠 KI-Copilot, Evidence-Ledger & Wissensbasis (§5)
- **Gemma 12B On-Premise:** Lokale Inferenz via Ollama (`http://localhost:11434`) ohne Datenabfluss.
- **DSGVO-Datenschutzfilter (PII-Maskierung):** Unkenntlichmachung von **IBANs, Kreditkarten, Passwörtern, API-Keys und Steuer-IDs**.
- **Evidence-Ledger (§5.2):** Gewichtete Faktenextraktion mit Quellen, Konfidenz (0–100 %) und Bestätigungs-Aktionen.
- **Wissensbasis / RAG (§5.5):** PV-Richtpreise, Speicher-Kapazitäten, UWG § 7 Regeln und Widerrufsbelehrungen nach § 355 BGB.
- **EU AI Act Observability (§5.5 / §20):** Live-Dashboard mit Latenzmetriken, Modellherkunft und PII-Audit-Log (Art. 50/52).

### 5. 📬 E-Mail Integration & Tagging-Engine (§4)
- **Split-View Inbox:** Zweispaltige Ansicht mit Filterleiste, Threading und Inline-Antwort-Editor.
- **Dynamisches Tagging & Flagging:** Filterung nach Standard-Tags (`🏷️ PV-Interessent`, `🏷️ Wärmepumpe`, `🏷️ Dringend`, `🏷️ Widerruf § 355`) und Anlegen beliebiger Custom-Tags.
- **Demo-Ingest-Simulator:** Einspeisen eingehender Testnachrichten direkt in der UI.

### 6. 📅 Kalender, Termine & Telefonie (§3.6 / §7.1a)
- **Termine mit ICS-Export:** Export von Kalenderterminen im Standard-Format **RFC 5545 `.ics`**.
- **Click-to-Call Telefonie:** Anruf-Overlay mit **Live-Gesprächstimer**, Dispositionsauswahl (`Erreicht`, `Nicht erreicht`) und Notizen.

### 7. 📊 Vertriebs-Report & 12-Monats-Forecast (§6.3)
- **Umsatzprognose:** Gegenüberstellung von gewichtetem und gesichertem Volumen über 12 Monate.
- **§ 355 BGB Widerrufsquote:** Statistische Erfassung ohne Verfälschung historischer Vertriebszahlen.

### 8. ⚙️ Systemeinstellungen & Benutzerverwaltung (§2 / §3.8 / §7.3)
- **Teamverwaltung mit Last-Admin-Schutz:** Rollenvergabe (`ADMIN`, `VERTRIEB`, `BACKOFFICE`), Einladungen und Kontosperren.
- **Sicherheit & 2FA:** RFC 6238 zeitbasierte Einmal-Passwörter (TOTP) mit QR-Code-Setup und Argon2id-Kennwortänderung.
- **Konnektoren & Webhooks:** Bearer-Token-geschützte Schnittstellen mit Zeitkonstanzprüfung (`subtle.ConstantTimeCompare`).
- **Revisionssicheres Backup & Restore-Drill:** JSON-Vollexport und automatisierte Datenbank-Integritätsprüfung.

---

## 🛡️ Sicherheitsarchitektur & Enterprise-Hardening

OpenLocalCRM wurde nach strengen Sicherheits- und Datenschutzstandards gehärtet:
- **Isolierter Demo-Modus:** Null-Datenbank-Architektur (`internal/db/demo`) hält Demo-Instanzen vollkommen getrennt von Produktivdaten.
- **Kryptographische Persistenz:** Ed25519-Schlüsselpaare für JWTs werden in einem dedizierten Docker-Volume (`crm_keys`) persistiert. Bei fehlerhafter Persistenz bricht die Produktion kontrolliert ab (`os.Exit(1)`), anstatt auf unsichere RAM-Schlüssel zurückzufallen.
- **Netzwerk-Isolation:** Der PostgreSQL-Port 5432 ist in der Produktion nicht nach außen exponiert, sondern ausschließlich im internen Docker-Netzwerk erreichbar.
- **Container-Sicherheit:** Go-Server und Worker laufen als unprivilegierter Benutzer `crmuser` (UID 10001) ohne Root-Rechte.
- **Robuste Zugriffskontrolle (RBAC & BOLA-Schutz):** Kritische Löschoperationen (`DELETE`) auf Kontakten, Firmen und Deals sind auf Administratoren beschränkt; Notiz-Autorenschaften sind kryptographisch an das JWT gebunden.
- **Input-Sanitization & SSRF-Schutz:** CSV-Formelschutz (`=`, `+`, `-`, `@`), RFC 1918 / Cloud-Metadaten / IPv4-mapped IPv6 IP-Filter gegen SSRF sowie XML-Tag-Kapselung gegen LLM-Prompt-Injections.
- **Brute-Force-Schutz:** Sliding-Window Rate Limiter (5 Fehlversuche / 5 Minuten) für alle Authentifizierungs-Endpunkte.

---

## 💾 Ressourceneffizienz (< 420 MB RAM Budget)

| Komponente | Basis-RAM | Ziel-Budget | Technologie |
| :--- | :--- | :--- | :--- |
| **`crm-proxy`** | ~ 15 MB | < 30 MB | Caddy v2 (Go-basiert, statisches Binary) |
| **`crm-server`** | ~ 25 MB | < 80 MB | Go 1.26+ + Chi + Eingebettetes React SPA (`//go:embed`) |
| **`crm-worker`** | ~ 20 MB | < 60 MB | Go 1.26+ + River Queue Worker Daemon |
| **`crm-db`** | ~ 60 MB | < 250 MB | PostgreSQL 16 (`pgvector`, `pg_trgm`) |
| **GESAMT** | **~ 120 MB** | **< 420 MB** | **Ideal für VPS (2–4 GB RAM / 1–10 Nutzer)** |

---

## 🚀 Schnellstart & Deployment

### Option 1: Demo-Modus (Sofort-Test in 30 Sekunden) ⚡

Ideal für Demos, Messen und schnelles Ausprobieren — läuft komplett in-memory (< 35 MB RAM) ohne externe Datenbank:

```bash
# 1. Repository klonen
git clone https://github.com/openlocalcrm/openlocalcrm.git
cd openlocalcrm

# 2. Standalone Demo-Stack starten
docker compose -f docker-compose.demo.yml up -d
```

Erreichbar unter:
- 🌐 **Web-Oberfläche:** [http://localhost](http://localhost) (oder direkt `:8080`)
- 🔑 **Vorkonfigurierte Demo-Accounts:**
  - Administrator: `admin@openlocalcrm.local` / `demo123`
  - Vertriebler: `vertrieb@openlocalcrm.local` / `demo123`
  - *(Inkl. Schnell-Login-Buttons auf der Anmeldeseite und Demo-Banner)*
- 📡 **REST API Health:** [http://localhost:8080/api/v1/health](http://localhost:8080/api/v1/health) (zeigt `"demo_mode": true`)

---

### Option 2: Starten mit Docker Compose (Produktion) 🛡️

Für den produktiven, revisionssicheren Einsatz mit PostgreSQL 16, persistenter Ed25519-Schlüsselverwaltung und Argon2id:

```bash
# 1. Repository klonen
git clone https://github.com/openlocalcrm/openlocalcrm.git
cd openlocalcrm

# 2. Produktions-Stack starten
make up

# 3. Live-Logs ansehen
make logs
```
Die Anwendung ist erreichbar unter:
- **Web-Oberfläche:** [http://localhost/](http://localhost/)
- **Initiales Admin-Konto:** `admin@openlocalcrm.local` (Kennwort wird via `INITIAL_ADMIN_PASSWORD` in `.env` gesetzt oder beim Erststart sicher generiert und in den Logs ausgegeben).
- **REST API Health:** [http://localhost:8080/api/v1/health](http://localhost:8080/api/v1/health) (`"demo_mode": false`)

---

### Option 3: 🪟 Windows 1-Klick Installer & Steuerungszentrale (`openlocalcrm-setup.exe`)

Für Windows-Anwender und Test-Nutzer steht mit dem Setup-Launcher (als Release-Download unter [GitHub Releases](https://github.com/boreddev1/openlocalcrm/releases) oder via `make build-all-launcher`) ein **eigenständiger, installationsfreier Assistent und Mini-Control-Center** bereit. Er automatisiert Systemprüfung, Installation, Container-Steuerung, Rebuilds und Backup-Handling – komplett ohne Terminal-Befehle!

```text
📁 openlocalcrm/
├── 📄 docker-compose.yml
├── 📄 Caddyfile
├── 📂 bin/
│   ├── 🚀 openlocalcrm-setup.exe        <-- Einfach per Doppelklick starten!
│   └── 🐞 openlocalcrm-setup-debug.exe  <-- Für Diagnose & Support mit sichtbarer Konsole
└── ...
```

---

#### ✨ Alle Features des Installers im Überblick

| Feature | Funktion | Details |
| :--- | :--- | :--- |
| **🔍 Intelligente System-Diagnose (Preflight)** | Automatische Prüfung aller Voraussetzungen | Prüft WSL2, Docker Desktop und Port-Belegungen vorab. |
| **⚡ 1-Klick WSL2 & Docker Setup** | Installation fehlender Komponenten | Installiert WSL2 (`wsl --install --no-distribution`) und Docker Desktop via `winget` per Klick mit automatischer UAC-Rechteerhöhung. |
| **🔄 Docker-Daemon Auto-Start** | Erkennt inaktive Docker-Dienste | Startet Docker Desktop bei Bedarf automatisch im Hintergrund und wartet auf Einsatzbereitschaft. |
| **🛡️ Kollisionsfreie Port-Prüfung** | Automatische Konflikterkennung | Prüft Port 80 und 443. Bei Konflikten (z. B. durch IIS, Skype oder VMware) schlägt der Assistent freie Alternativports (z. B. 8080) vor. |
| **🧙 4-Stufen Setup-Assistent** | Geführte Erstkonfiguration | Generiert sichere Passwörter, konfiguriert E-Mail, Port, Demo-Modus und bindet KI-Gateways ein. |
| **🧠 KI-Gateway Konfiguration** | Flexible LLM-Anbindung mit Live-Test | Unterstützt lokales Ollama (`gemma4:12b`), OpenAI, Anthropic und Groq inklusive 1-Klick-Verbindungstest ("Verbindung prüfen"). |
| **📊 Interaktive Steuerungszentrale** | Zentrales Dashboard im Browser | Zeigt Live-Status aller Container (`crm-proxy`, `crm-server`, `crm-worker`, `crm-db`) mit Uptime und Health-Checks. |
| **🎮 Stack-Steuerung** | Starten, Stoppen, Neustarten | Ermöglicht das Starten (`▶️`), Neustarten (`🔄`) und Stoppen (`⏹️`) aller Container per Mausklick. |
| **🔄 1-Klick System-Update (Rebuild)** | Code-Aktualisierung ohne Datenverlust | Erstellt vorab ein automatisches Sicherheits-Backup und baut alle Container (`docker compose up -d --build --remove-orphans`) mit neuem Code neu. |
| **📦 Automatische Container-Adoption** | Erkennt bestehende Container | Erkennt bei neuen ZIP-Versionen bereits laufende oder gestoppte Container und übernimmt deren Konfiguration ohne Passwort-Diskrepanzen. |
| **💾 1-Klick Backups** | Sofortige Datenbank-Sicherung | Erstellt vollständige DDL/DML-Dumps der PostgreSQL-Datenbank (`openlocalcrm_backup_YYYY-MM-DD_HHMMSS.sql`). |
| **🛡️ Persistente Doppel-Sicherung** | Schutz vor versehentlichem Löschen | Backups werden im Projektordner (`backups/`) UND redundant unter `%LOCALAPPDATA%\openlocalcrm\backups` gesichert. |
| **⬇️ Backup-Download** | Direkt-Download über den Browser | Jedes Backup kann per Klick (`⬇️ Download`) sicher auf den lokalen Rechner heruntergeladen werden. |
| **⬆️ Backup-Import** | Beliebige SQL-Dumps importieren | Einfaches Hochladen externer Backups per Dateiauswahl. |
| **🔄 5-Stufen Safe-Restore mit Vorwärts-Migration** | Sichere Wiederherstellung | Automatischer Sicherheits-Snapshot vor Restore, Schema-Bereinigung, DDL-Import und automatisches Ausführen neuer Vorwärts-Migrationen (`schema_migrations`). |
| **🔑 Admin-Passwort Reset** | Notfall-Zugang & Entsperrung | Setzt das Administrator-Passwort direkt in PostgreSQL mit Argon2id neu, deaktiviert 2FA/TOTP und meldet alte Sitzungen ab. |
| **⚙️ Konfiguration anpassen** | Einstellungen ändern ohne Datenverlust | Ändern von Port, Admin-E-Mail oder KI-Anbieter unter garantierter Beibehaltung des PostgreSQL-Passworts und Konnektor-Tokens. |
| **📜 Live-Terminal** | Echtzeit-Protokollierung | Verfolgt alle Docker-, Build- und Restore-Schritte live via Server-Sent Events (SSE). |

---

#### 📖 Schritt-für-Schritt-Bedienung (Installer)

1. Laden Sie das aktuelle Release-Archiv (`openlocalcrm.zip`) oder `openlocalcrm-setup.exe` herunter.
2. Starten Sie **`openlocalcrm-setup.exe`** per Doppelklick.
3. Ihr Standardbrowser öffnet sich automatisch unter `http://127.0.0.1:9099`.
4. Der Preflight-Check führt Sie durch WSL2, Docker Desktop und Port-Zuweisung.
5. Vergeben Sie Administrator-Konto und KI-Verbindung und klicken Sie auf **"CRM jetzt installieren & starten"**.

---

#### 💾 Backup & Disaster Recovery (Installer)
- **Sicherheits-Backup erstellen:** In der Steuerungszentrale auf **"💾 1-Klick Backup erstellen"** klicken.
- **Backup wiederherstellen:** Backup in der Tabelle wählen oder `.sql`-Datei hochladen und **"Wiederherstellen"** bestätigen.
- **Admin-Passwort vergessen:** Button **"🔑 Admin-Passwort zurücksetzen"** in der Steuerungszentrale nutzen.

---

#### ❓ Troubleshooting & Support (Windows)
- **Port 80 belegt:** Preflight erkennt Port-Kollisionen automatisch; Port im Setup auf `8080` oder `3000` umstellen.
- **WSL2 Virtualisierung:** Im BIOS/UEFI *Intel VT-x* bzw. *AMD SVM* aktivieren.
- **Diagnose-Logs:** `openlocalcrm-setup.log` im Projektverzeichnis oder `openlocalcrm-setup-debug.exe` mit sichtbarer Konsole nutzen.

---

### Option 4: Lokale Entwicklung mit `make` 💻

```bash
# Frontend und Server-Binary bauen
make build

# Alle 27 Playwright E2E UI-Tests ausführen
make test-e2e

# Go Backend Unit-Tests ausführen
make test

# Open-Source-Lizenzen scannen
make check-licenses

# Server lokal starten
make run-local
```

---

## 🧪 Test-Suite & Qualitätssicherung

Das Projekt verfügt über eine umfassende Testabdeckung für Backend und Frontend:
- **64 Go-Testdateien** für Unit- & Integrationsprüfungen aller Services und API-Routen (`make test`).
- **27 automatisierte Playwright E2E-Tests** (`make test-e2e`) für vollständige UI- und User-Journey-Verifikation.
- **Pre-Commit & Pre-Push Hooks** (`make check`) zur Sicherstellung von Formatierung (`gofmt`), statischer Code-Analyse (`go vet`), TypeScript-Prüfung und Lizenz-Compliance.
- **Aktueller CI-Status:** Siehe [![Playwright E2E](https://img.shields.io/badge/Playwright%20E2E-27%20passed%20(100%25)-2EAD33?style=flat&logo=playwright)](https://playwright.dev/) oben.

---

## 📚 Vollständige Dokumentation & Wiki

| Leitfaden / Dokument | Pfad | Beschreibung |
|---|---|---|
| 🚀 **Getting Started Guide** | [docs/GETTING_STARTED.md](docs/GETTING_STARTED.md) | Schritt-für-Schritt Schnellstart, Erst-Setup & Einstieg in 60 Sekunden |
| 📖 **User Guide & Sales Playbooks** | [docs/USER_GUIDE_AND_PLAYBOOKS.md](docs/USER_GUIDE_AND_PLAYBOOKS.md) | Praxis-Workflows für E-Mail Triage, D2D-Vertrieb, Telefonie & § 355 BGB Widerruf |
| 🔄 **End-to-End Prozess-Beispiele** | [docs/END_TO_END_PROCESS_EXAMPLES.md](docs/END_TO_END_PROCESS_EXAMPLES.md) | Konkrete Abläufe: Inbound PV-Lifecycle, D2D Haustür-Akquise, Service & SLA |
| ❓ **FAQ & Fehlerbehebung** | [docs/FAQ_AND_TROUBLESHOOTING.md](docs/FAQ_AND_TROUBLESHOOTING.md) | Häufige Fragen zu KI, Single-Tenant, D2D-Pins, Port-Konflikten & Migrationen |
| 📡 **REST API & Webhooks** | [docs/API_REFERENCE.md](docs/API_REFERENCE.md) | Endpunkt-Dokumentation mit cURL-Befehlen, JWT & Lead-Intake Webhooks |
| 🧠 **KI-Copilot & EU AI Act** | [docs/AI_COPILOT_AND_COMPLIANCE.md](docs/AI_COPILOT_AND_COMPLIANCE.md) | Gemma 12B Inferenz, Evidence-Ledger, PII-Filter & Revisionssicheres Audit-Log |
| ⚙️ **Konfiguration & Umgebung** | [docs/CONFIGURATION_AND_ENV.md](docs/CONFIGURATION_AND_ENV.md) | Alle `.env`-Parameter, PostgreSQL-Tuning & lokales Ollama-Setup |
| 📐 **Systemarchitektur** | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Detaillierte Topologie, StorageService, SSE Streaming Hub & Datenmodell |
| 🗺️ **D2D-Vertikale & Konnektoren** | [docs/D2D_AND_CONNECTORS.md](docs/D2D_AND_CONNECTORS.md) | Leaflet/OSM Kartengestaltung, Energiedaten & Partner-Konnektoren |
| 🚢 **Produktions-Deployment** | [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Docker Compose, GHCR Multi-Arch Images (`ghcr.io`) & Restore-Drill |
| ⚖️ **Compliance & Lizenzen** | [docs/COMPLIANCE_AND_LICENSES.md](docs/COMPLIANCE_AND_LICENSES.md) | DSGVO Art. 20 Export, UWG § 7 Consent-Tracking & 100% MIT-Audit |

---

## 📄 Lizenz

Dieses Projekt ist unter der **[MIT-Lizenz](LICENSE)** lizenziert. Alle verwendeten Drittanbieter-Bibliotheken sind in [THIRD-PARTY-LICENSES.csv](THIRD-PARTY-LICENSES.csv) dokumentiert und entsprechen den strengen Open-Source-Vorgaben nach §22 der Fachspezifikation.

