# 🚢 Produktions-Deployment & Wartungs-Leitfaden

Dieses Dokument beschreibt das Setup, den Betrieb und die Wartung von **OpenLocalCRM (Single-Tenant v3)** in hochverfügbaren Produktivumgebungen.

---

## 1. Systemvoraussetzungen

- **CPU:** 1–2 vCPUs
- **RAM:** 2–4 GB RAM (Stack benötigt im Kernbetrieb **< 420 MB**)
- **Disk:** 20 GB NVMe/SSD
- **Betriebssystem:** Debian 12, Ubuntu 24.04 / 22.04 LTS, AlmaLinux 9 oder macOS
- **Container-Runtime:** Docker Engine 24+ & Docker Compose v2+ oder Podman

---

## 2. Schnelles Produktiv-Setup mit `make`

```bash
# 1. Repository klonen
git clone https://github.com/boreddev1/openlocalcrm.git
cd openlocalcrm

# 2. Konfiguration anlegen
cp .env.example .env

# 3. Stack starten
make up

# 4. Live-Logs überwachen
make logs
```

---

## 3. Automatische Datenbank-Migrationen

Der Server führt beim Start automatisch alle im Binary eingebetteten SQL-Migrationen (`00001` bis `00011`) aus:
- **Tabelle:** `schema_migrations`
- **Transaktionssicherheit:** Jede Migration wird in einer eigenen PostgreSQL-Transaktion angewendet.
- **Rollback-Schutz:** Fehlgeschlagene Migrationen führen zu einem kontrollierten Abbruch, ohne Altbestände zu beschädigen.

---

## 4. Container-Images aus der GitHub Container Registry (GHCR)

Für Produktivsysteme können vorgebaute Multi-Arch Images (`linux/amd64` und `linux/arm64`) direkt bezogen werden:

```yaml
# Beispiel docker-compose.yml mit GHCR Images
services:
  server:
    image: ghcr.io/boreddev1/openlocalcrm/server:latest
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgres://crm_user:${DB_PASSWORD:-crm_pass}@db:5432/crm_db?sslmode=disable
      - PORT=8080
    depends_on:
      db:
        condition: service_healthy

  worker:
    image: ghcr.io/boreddev1/openlocalcrm/worker:latest
    restart: unless-stopped
    environment:
      - DATABASE_URL=postgres://crm_user:${DB_PASSWORD:-crm_pass}@db:5432/crm_db?sslmode=disable
```

---

## 5. Healthchecks & Monitoring

| Endpunkt | Methode | Zweck | Erwarteter Status |
|---|---|---|---|
| `/api/v1/health` | `GET` | Server & App-Liveness | `200 OK` (`{"status":"healthy","demo_mode":false}`) |
| `/events/stream` | `GET` | Authentifizierter SSE Event Stream | `200 OK` (`text/event-stream` mit Bearer-Token) |

---

## 6. Datensicherung & Restore-Drill (§8.7)

### REST-API Backup- & Konsistenz-Funktionen (Admin-Berechtigung erforderlich)

1. **Automatisierter Konsistenz-Check (Restore-Drill):**
   ```bash
   curl -X POST http://localhost:8080/api/v1/backup/drill \
     -H "Authorization: Bearer <ADMIN_JWT_TOKEN>"
   ```
   Überprüft alle Tabellen-Zeilenzahlen und die referentielle Integrität.

2. **Vollständiger CRM JSON-Export (Disaster Recovery):**
   ```bash
   curl -X GET http://localhost:8080/api/v1/backup/export \
     -H "Authorization: Bearer <ADMIN_JWT_TOKEN>" > crm_full_export_$(date +%Y%m%d).json
   ```

### PostgreSQL Datenbank-Backup & Wiederherstellung

```bash
# Datenbank-Dump
docker compose exec db pg_dump -U crm_user crm_db > backup_$(date +%Y%m%d_%H%M%S).sql

# Dateispeicher-Backup
tar -czvf storage_backup_$(date +%Y%m%d).tar.gz /data/storage

# Wiederherstellungstest (Restore-Drill)
docker compose exec -T db psql -U crm_user crm_db < backup_20260825.sql
```

---

## 7. Standalone Demo-Bereitstellung (Messen & Kundendemos)

Für Präsentationen ohne PostgreSQL-Server oder Persistenzanforderungen:

```bash
# Startet isolierten In-Memory Demo-Modus
docker compose -f docker-compose.demo.yml up -d
```
- Vollständig isolierter In-Memory Speicher (`internal/db/demo`)
- Keine Persistenz, kein PostgreSQL-Container nötig (< 35 MB RAM)
- Demo-Zugangsdaten: `admin@openlocalcrm.local` / `demo123` und `vertrieb@openlocalcrm.local` / `demo123` (Legacy-Aliase: `admin@mavalio.local` / `vertrieb@mavalio.local`)

---

## 8. Sicherheits-Hardening Checkliste für Produktion

- [x] **PostgreSQL Port 5432 geschlossen:** Nicht am Host exponiert, nur im Docker-Bridge-Netzwerk erreichbar.
- [x] **Kryptographie-Volume:** Volume `crm_keys` für Ed25519-Schlüsselpaar fest gemountet.
- [x] **Non-Root Execution:** Container läuft unter Benutzer `crmuser` (UID 10001).
- [x] **Sicherheits-Header in Caddy:** HSTS, CSP (`frame-ancestors 'none'`), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`.
- [x] **RBAC & BOLA Schutz:** Lösch- und Benutzerverwaltungs-Endpunkte durch Administrator-Rolle und Last-Admin-Schutz gesichert.
