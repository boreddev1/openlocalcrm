# ⚙️ Konfigurations-Handbuch & Umgebungsvariablen

Dieses Handbuch beschreibt alle Konfigurationsparameter, Umgebungsvariablen (`.env`) und Leistungsoptimierungen für den Produktivbetrieb von **OpenLocalCRM**.

---

## 📋 Übersicht der Umgebungsvariablen

| Variable | Standardwert | Pflicht? | Beschreibung |
|---|---|---|---|
| `PORT` | `8080` | Nein | Port des Go API-Servers |
| `DOMAIN` | `localhost` | Nein | Hauptdomain für Caddy-TLS & Cookie-Domain |
| `DATABASE_URL` | `postgres://...` | Ja (Prod) | Verbindungs-URI zur PostgreSQL 16 Datenbank (entfällt bei `DEMO_MODE=true`) |
| `DB_HOST` | `crm-db` | Ja (Docker) | Hostname des PostgreSQL-Containers |
| `DB_PORT` | `5432` | Nein | Port des PostgreSQL-Servers |
| `DB_NAME` | `crm_db` | Ja (Docker) | Name der PostgreSQL-Datenbank |
| `DB_USER` | `crm_user` | Ja (Docker) | PostgreSQL-Benutzername |
| `DB_PASSWORD` | — | Ja (Prod) | Pflichtpasswort für PostgreSQL (in `.env` definieren) |
| `DEMO_MODE` | `false` | Nein | Bei `true` wird der In-Memory RAM Querier gestartet; keine externe DB nötig |
| `INITIAL_ADMIN_EMAIL` | `admin@openlocalcrm.local` | Nein | E-Mail des initialen Administrators |
| `INITIAL_ADMIN_PASSWORD` | — | Nein | Initiales Passwort für den Admin beim Erststart (wird sonst sicher generiert) |
| `STORAGE_PATH` | `/data/storage` | Nein | Lokales Verzeichnis für Dateiuploads & E-Mail-Anhänge (Kompatibilitäts-Alias: `STORAGE_LOCAL_DIR`) |
| `JWT_SECRET_KEY_PATH` | `/app/keys/ed25519.key` | Ja (Prod) | Pfad zum persistenten Ed25519-Schlüssel (Kompatibilitäts-Alias: `JWT_PRIVATE_KEY_PATH`) |
| `CONNECTOR_API_TOKEN` | — | Ja (Prod) | Geheimer API-Schlüssel für Lead-Intake Webhooks (Bearer Auth) |
| `AI_PROVIDER` | `ollama` | Nein | KI-Inferenz-Provider (`ollama`, `openai`, `gemini`) |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Nein | URL der lokalen Ollama-Instanz |
| `OLLAMA_MODEL` | `gemma4:12b` | Nein | Verwendetes Sprachmodell (z. B. `gemma4:12b`, `qwen3:8b`) |
| `AI_API_KEY` | — | Nein | API-Key bei Nutzung externer Provider (OpenAI, Gemini) |
| `AI_BASE_URL` | — | Nein | Optionale Custom-Base-URL für OpenAI-kompatible Proxies |
| `LOG_LEVEL` | `info` | Nein | Log-Level (`debug`, `info`, `warn`, `error`) |

> [!WARNING]
> **Produktions-Schlüsselsicherheit:**
> In der Produktion (`DEMO_MODE=false`) bricht der Server mit einem fatalen Fehler ab, falls die Ed25519-Schlüsseldateien nicht persistiert werden können. Es gibt keinen stillen RAM-Fallback im Produktivmodus, um Session-Verluste und Token-Fälschungen nach Container-Neustarts auszuschließen. Stellen Sie sicher, dass `/app/keys` als Docker-Volume (`crm_keys`) gemountet ist.

---

## 🗄️ PostgreSQL 16 & pgvector Optimierung

Für VPS-Instanzen mit 2–4 GB RAM empfehlen sich folgende Einstellungen in `postgresql.conf`:

```ini
# Memory Configuration (< 250 MB RAM Budget)
shared_buffers = 128MB
effective_cache_size = 384MB
work_mem = 4MB
maintenance_work_mem = 32MB

# WAL & Checkpoints
max_wal_size = 1GB
min_wal_size = 80MB
checkpoint_completion_target = 0.9

# Extensions
shared_preload_libraries = 'vector'
```

---

## 🧠 Lokales Ollama Setup für Offline-KI

1. **Ollama installieren:**
   ```bash
   curl -fsSL https://ollama.com/install.sh | sh
   ```
2. **Gemma 12B Modell laden:**
   ```bash
   ollama pull gemma4:12b
   ```
3. **Starten:**
   ```bash
   ollama serve
   ```
4. OpenLocalCRM verbindet sich automatisch mit `http://localhost:11434` und nutzt das Modell für E-Mail-Triage, Fakten-Extraktion und Copilot-Chat.

---

## 🌐 Caddy Reverse Proxy & Eigene Domain

In der Datei [`Caddyfile`](../Caddyfile) Ihre Domain eintragen:

```caddyfile
crm.ihre-firma.de {
    encode gzip zstd

    # API & SSE Streaming (Unbuffered)
    handle /events/* {
        reverse_proxy server:8080 {
            flush_interval -1
        }
    }

    handle /api/* {
        reverse_proxy server:8080
    }

    # Embedded React Frontend
    handle {
        reverse_proxy server:8080
    }
}
```
Caddy bezieht und erneuert automatisch kostenlose **Let's Encrypt SSL-Zertifikate**.
