# Technische Architekturspezifikation v3 — OpenLocalCRM (Single-Tenant)

> **Status:** Genehmigt (User-Review)  
> **Datum:** 2026-08-24  
> **Basis:** `2026-08-22-fachspezifikation-v3-single-tenant-design.md`  
> **Ziel-Umgebung:** Docker / Podman Stack (< 450 MB RAM Basis, Budget: < 4 GB)

---

## 1. Systemübersicht & Design-Prinzipien

OpenLocalCRM v3 ist ein schlankes, generalistisches und KI-gestütztes Single-Tenant-CRM für 1–10 Nutzer mit vertikalem Fokus auf Energie- und Feldvertrieb (D2D).

### 1.1 Kern-Leitlinien
1. **Ressourceneffizienz:** Der Kern-Stack benötigt im Normalbetrieb **unter 450 MB RAM** und läuft problemlos auf kleinsten VPS-Instanzen.
2. **Single-Tenant-Einfachheit:** 1 Organisation, 2 Rollen (`Admin`, `Benutzer`), keine RLS- oder Multi-Tenancy-Komplexität.
3. **Strikte Trennung & Stabilität:** HTTP/Web-Traffic und asynchrone Hintergrund-Worker (IMAP-Sync, KI-Jobs) laufen in getrennten Containern aus derselben Go-Codebasis.
4. **All-in-One Data Engine:** PostgreSQL 16 übernimmt relationale Daten, Volltextsuche (`pg_trgm`/`tsvector`), Vektorsuche (`pgvector`) und die transaktionale Job-Queue (`riverqueue`).
5. **KI mit Prompt Guards & Human-in-the-Loop:** Alle KI-Prompts werden an kleinen Modellen (z. B. Gemma 12B) verprobt und mit vorgeschalteten Injection- & PII-Guards abgesichert.

---

## 2. Container-Topologie & Ressourcenbudget

### 2.1 Standard Docker / Podman Stack

```mermaid
graph TD
    Client["Browser / Mobile Client"] -->|HTTPS :443 / HTTP :80| Caddy["caddy\nReverse Proxy\n(Auto-TLS, Security Headers)\nRAM: ~30 MB"]
    
    subgraph CoreStack ["Core Stack (Budget: < 450 MB RAM)"]
        Caddy -->|/api/*, /events (SSE), /*| App["crm-server\nGo API & Web Server\n(Embedded React SPA, Auth, REST, SSE)\nRAM: ~50 MB"]
        
        App -->|pgxpool :5432| DB[("crm-db\nPostgreSQL 16 + pgvector\n(DB, Vectors, FTS, River Queue)\nRAM: ~250-300 MB")]
        
        Worker["crm-worker\nGo Background Worker\n(River Queue, IMAP Polling, AI Pipeline)\nRAM: ~50 MB"] -->|pgxpool :5432| DB
        
        App -->|Read/Write /data/storage| Storage[("Docker Volume\ncrm-storage")]
        Worker -->|Read/Write /data/storage| Storage
    end

    subgraph OptionalProfiles ["Optionale Compose-Profile (separates Ressourcenkontingent)"]
        Worker -.->|--profile local-ai| Ollama["ollama / vllm\n(Gemma 12B / Lokale Inferenz)"]
        Worker -.->|--profile meta-search| SearXNG["searxng\n(Web-Research)"]
    end

    App -.->|HTTPS API| ExtLLM["Externe LLM-APIs\n(Claude, OpenAI, Gemini, Mistral)"]
    Worker -.->|HTTPS API| ExtLLM
    Worker -.->|IMAP / SMTP / Graph| Mail["E-Mail Provider\n(M365, Google, Standard IMAP)"]
```

### 2.2 Ressourcen-Aufstellung

| Container | Image / Basis | Aufgabe | RAM-Bedarf |
|---|---|---|---|
| `crm-proxy` | `caddy:2-alpine` | Auto-TLS (Let's Encrypt), Reverse Proxy, Rate Limiting, Gzip/Zstd | ~30 MB |
| `crm-server` | `scratch` / Alpine (Go Binary) | REST-API, SSE-Event-Hub, Static Asset Serving (React SPA), Auth | ~50 MB |
| `crm-worker` | `scratch` / Alpine (Go Binary) | River Job Execution, IMAP-IDLE/Polling, KI-Pipeline, Cronjobs | ~50 MB |
| `crm-db` | `pgvector/pgvector:pg16` | PostgreSQL 16 mit pgvector, pg_trgm, FTS und River-Tabellen | ~250–300 MB |
| **Gesamt Basis-Stack** | | | **~380–430 MB** |

---

## 3. Backend-Architektur (Go)

### 3.1 Projektstruktur (Clean Architecture)

```
├── cmd/
│   ├── server/          # Einstiegspunkt für den API- & Webserver
│   └── worker/          # Einstiegspunkt für den Hintergrund-Worker
├── internal/
│   ├── core/            # Domänen-Entitäten & Geschäftslogik
│   │   ├── contact/     # Kontakte, Firmen, Beziehungen
│   │   ├── deal/        # Pipelines, Phasen, Setter/Closer
│   │   ├── email/       # IMAP/SMTP/Graph Sync, E-Mail-Parsing
│   │   ├── workflow/    # Automatisierungsregeln & Aktionen
│   │   ├── document/    # Dokumentenverwaltung & Extraktion
│   │   └── connector/   # Konnektoren (z.B. Energie-Tarifrechner, Telefonie)
│   ├── ai/              # KI-Gateway & Guardrails
│   │   ├── gateway/     # Provider (Claude, OpenAI, Gemini, Ollama)
│   │   ├── guards/      # Injection-Detection, PII-Maskierung
│   │   ├── prompts/     # Strukturierte Prompts (optimiert für Gemma 12B)
│   │   └── parser/      # JSON-Schema-Validierung
│   ├── db/              # sqlc generierte Abfragen und Modelle
│   │   ├── queries/     # .sql Abfragedateien
│   │   ├── migrations/  # goose / golang-migrate SQL-Dateien
│   │   └── db.go        # Generierter Go-Code (pgx)
│   ├── queue/           # River-Queue Definitionen & Worker-Handler
│   ├── storage/         # StorageService (Local Disk Volume & S3-Adapter)
│   ├── auth/            # JWT (Ed25519), TOTP 2FA, API-Token Hashing
│   └── sse/             # Server-Sent Events Dispatcher
└── web/                 # React Frontend (TypeScript + Vite)
```

### 3.2 Datenzugriffsschicht mit `sqlc` & `pgx`
- **Kein ORM:** 100 % typsicheres SQL ohne Reflection-Overhead zur Laufzeit.
- **Connection-Pooling:** `jackc/pgx/v5/pgxpool` für optimierte Wiederverwendung von DB-Verbindungen.
- **Migrationen:** Nummerierte SQL-Migrationen via `goose` beim Server-Start.

### 3.3 Asynchrone Jobs mit `riverqueue`
- **Transaktional:** Jobs werden innerhalb derselben Postgres-Transaktion wie die CRM-Daten geschrieben (ACID).
- **Beispiele für Worker-Jobs:**
  - `EmailSyncJob`: Periodischer Abruf / IDLE-Listener für konfigurierte IMAP-Konten.
  - `EmailTriageJob`: KI-Klassifikation und Entwurfserstellung für eingehende E-Mails.
  - `DocumentExtractJob`: Textextraktion aus PDFs/DOCX und Erzeugung von Vektor-Embeddings.
  - `WorkflowStepJob`: Ausführung konfigurierter Folgeaktionen.

---

## 4. Frontend-Architektur (React + TypeScript)

### 4.1 Technische Basis
- **Framework & Bundler:** React 18+ mit TypeScript und Vite.
- **Auslieferung:** Der Vite-Build erzeugt statische Dateien in `web/dist/`, welche zur Buildzeit via `//go:embed` in das Go-Binary `crm-server` eingebunden werden.
- **Routing & State:** `react-router` für Navigation, `@tanstack/react-query` für Server-State, Caching und Optimistic UI Updates.

### 4.2 Kern-Komponenten & Bibliotheken
- **Kanban-Boards:** `@dnd-kit/core` für Drag & Drop bei Deal-Pipelines und Setter/Closer-Übersichten.
- **E-Mail-Posteingang:** Split-View mit `@tiptap/react` für den Rich-Text-Editor (inklusive Einbettung von KI-Vorschlägen).
- **Außendienst / D2D-Karten:** `leaflet` und `react-leaflet` mit OpenStreetMap-Kacheln für Adress-Cluster und Routenansichten.
- **Design & UI-Primitiven:** Tailwind CSS mit Radix UI Primitives für Barrierefreiheit und Theme-Support (Dark/Light).

---

## 5. KI- & Agenten-Architektur

### 5.1 Provider-Agnostisches LLM-Gateway
Das Gateway stellt ein einheitliches Go-Interface bereit:
```go
type LLMGateway interface {
    GenerateStructured(ctx context.Context, prompt string, schema JSONSchema) (string, error)
    StreamCompletion(ctx context.Context, prompt string, out chan<- string) error
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
}
```

### 5.2 Prompt-Guards & Sicherheitsstufen (von Tag 1)
1. **Input-Sanitization:** Eingehende E-Mail-Texte und Kundendaten werden vor der Übergabe an das Prompt durch Regex- und Delimiter-basierte Filter isoliert (`<user_data>`-Tags mit striktem Escape).
2. **PII-Masking:** Personenbezogene Ausweisdaten, sensible Bankdaten oder Kennwörter werden vor der API-Übertragung pseudonymisiert.
3. **Structured Outputs:** Jede Klassifikation, Triage und Entwurfsgenerierung verlangt ein striktes JSON-Schema. Das Go-Backend validiert das JSON gegen Go-Structs vor jeglicher Verarbeitung.
4. **Human-in-the-Loop:** Aktionen mit Außenwirkung (E-Mail senden, Deal-Status ändern) werden als Status `DRAFT_PENDING_APPROVAL` in der DB gespeichert und erfordern den Benutzerklick in der UI.

### 5.3 Modell-Verprobung auf kleinen Modellen
- Sämtliche System-Prompts, Tool-Calling-Strukturen und Triage-Regeln werden primär auf **Gemma 12B** (und Mistral Small) verprobt und optimiert, sodass Single-Tenant-Instanzen auch mit lokalen oder kostengünstigen Modellen maximale Zuverlässigkeit erreichen.

---

## 6. Authentifizierung, Security & Datenhaltung

### 6.1 Authentifizierung & Sessions
- **Stateless JWTs:** Ed25519-signierte Tokens mit kurzer Gültigkeit (15 Minuten) im `HttpOnly`-, `Secure`-, `SameSite=Lax`-Cookie.
- **Refresh-Token-Rotation:** Langläufer-Refresh-Tokens mit automatischer Rotation in der Datenbank.
- **2FA:** RFC 6238 TOTP (Google Authenticator, Apple Passwords, 1Password).
- **Passwort-Hashing:** Argon2id mit sicheren Standardparametern.
- **API-Tokens:** SHA-256 gehashte Tokens für Konnektoren und Webhooks mit konfigurierbaren Scopes.

### 6.2 Dateispeicherung & Dokumentenextraktion
- **Ablage:** `StorageService`-Interface. Standardmäßig auf gemountetes Docker-Volume (`/data/storage`), umschaltbar auf S3/R2 via Konfiguration.
- **Textextraktion:** Go-native Parser (`ledongthuc/pdf`, standard Text-/HTML-Parser) für Volltext- und Vektorextraktion ohne schwere externe Container.

### 6.3 Echtzeit & Benachrichtigungen
- **Server-Sent Events (SSE):** `/api/v1/events/stream` liefert Live-Events (E-Mail eingegangen, Todo zugewiesen, KI-Streaming-Token) über HTTP/2.

---

## 7. Verifikationsplan

### 7.1 Automatisierte Tests
- **Go Unit- & Integrationstests:** Testen von Core-Logik, Guardrails, PII-Maskierung und SQL-Abfragen gegen eine Postgres-Testdatenbank (`testcontainers-go`).
- **Prompt-Evaluierung:** CI-Suite mit Testfällen zur Verifikation der Prompt-Robustheit gegen Gemma 12B.
- **Frontend-Tests:** Vitest & Playwright für E2E-Tests kritischer Flows (Login, Kanban Drag & Drop, E-Mail-Draft Freigabe).

### 7.2 Ressourcen- & Last-Verifikation
- `docker stats` Überprüfung: Der Basis-Stack (Proxy + Server + Worker + DB) muss unter Last mit 10 parallelen Sessions < 500 MB RAM bleiben.
