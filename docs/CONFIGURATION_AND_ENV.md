# ⚙️ Konfigurations-Handbuch & Umgebungsvariablen

Dieses Handbuch beschreibt alle Konfigurationsparameter, Umgebungsvariablen (`.env`) und Leistungsoptimierungen für den Produktivbetrieb von **OpenLocalCRM**.

---

## 📋 Übersicht der Umgebungsvariablen

| Variable | Standardwert | Pflicht? | Beschreibung |
|---|---|---|---|
| `PORT` | `8080` | Nein | Port des Go API-Servers |
| `DATABASE_URL` | `postgres://...` | Ja (Prod) | Verbindungs-URI zur PostgreSQL 16 Datenbank (entfällt bei `DEMO_MODE=true`) |
| `DEMO_MODE` | `false` | Nein | Bei `true` wird der In-Memory RAM Querier gestartet; keine externe DB nötig |
| `INITIAL_ADMIN_PASSWORD` | — | Nein | Initiales Passwort für `admin@openlocalcrm.local` beim Erststart (wird sonst sicher generiert) |
| `STORAGE_PATH` | `/data/storage` | Nein | Lokales Verzeichnis für Dateiuploads & E-Mail-Anhänge |
| `CONNECTOR_API_TOKEN` | — | Ja (Prod) | Geheimer API-Schlüssel für Lead-Intake Webhooks (Bearer Auth) |
| `AI_PROVIDER` | `ollama` | Nein | KI-Inferenz-Provider (`ollama`, `openai`, `gemini`) |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Nein | URL der lokalen Ollama-Instanz |
| `OLLAMA_MODEL` | `gemma4:12b` | Nein | Verwendetes Sprachmodell (z. B. `gemma4:12b`, `qwen3:8b`) |
| `JWT_PRIVATE_KEY_PATH` | `/app/keys/ed25519.key` | Ja (Prod) | Pfad zum persistenten privaten Ed25519-Schlüssel (muss auf gemountetem Volume liegen) |
| `JWT_PUBLIC_KEY_PATH` | `/app/keys/ed25519.pub` | Ja (Prod) | Pfad zum persistenten öffentlichen Ed25519-Schlüssel |
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
