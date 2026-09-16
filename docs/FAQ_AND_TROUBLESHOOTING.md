# ❓ FAQ & Fehlerbehebung (Troubleshooting)

Häufig gestellte Fragen (FAQ), Best Practices und Lösungen für typische Problemstellungen im Betrieb von **OpenLocalCRM (Single-Tenant v3)**.

---

## 📑 Inhaltsverzeichnis
1. [Allgemeines & Architektur](#1-allgemeines--architektur)
2. [KI-Copilot & LLM-Inferenz (Gemma 12B)](#2-ki-copilot--llm-inferenz-gemma-12b)
3. [D2D Feldvertrieb & Gebietskarte](#3-d2d-feldvertrieb--gebietskarte)
4. [E-Mail, Kalender & Telefonie](#4-e-mail-kalender--telefonie)
5. [Automatisierung & Human-in-the-Loop (HITL)](#5-automatisierung--human-in-the-loop-hitl)
6. [Diagnose & Fehlerbehebung (Troubleshooting)](#6-diagnose--fehlerbehebung-troubleshooting)

---

## 1. Allgemeines & Architektur

### Q: Was bedeutet "Single-Tenant" und warum wurde dieser Ansatz gewählt?
**A:** Bei Single-Tenant läuft für Ihre Organisation eine eigene, isolierte Instanz mit eigener PostgreSQL-Datenbank und eigenem Dateispeicher. Es gibt keine Mandantenmischung ("Multi-Tenancy"), keine komplexen Berechtigungsmatrizen und kein Risiko von Datenlecks zwischen Unternehmen. Dies garantiert maximale Datensouveränität und erfüllt die strengen Anforderungen der DSGVO.

### Q: Wie viele Ressourcen benötigt das CRM im laufenden Betrieb?
**A:** Das gesamte System ist auf ein **Ultra-Low Memory Budget von < 420 MB RAM** optimiert:
- Caddy Proxy: ~ 15 MB
- Go API Server (inkl. eingebettetem React Frontend): ~ 25 MB
- Go River Worker Daemon: ~ 20 MB
- PostgreSQL 16: ~ 60–150 MB
Ein kleiner VPS mit 2–4 GB RAM reicht für 1–10 gleichzeitige Benutzer vollkommen aus.

### Q: Welche Rollen gibt es im System?
**A:** Nach §2 der Fachspezifikation gibt es genau zwei Rollen:
1. `Admin`: Vollzugriff, Einrichtung von E-Mail-Konten, Konnektoren, Custom Fields, Workflows und 2FA.
2. `Benutzer`: Vertriebler/Mitarbeiter (Zugriff auf Kontakte, Firmen, Deals, Kalender, Karte, E-Mails, KI-Copilot und HITL-Freigaben).

---

## 2. KI-Copilot & LLM-Inferenz (Gemma 12B)

### Q: Werden meine Kundendaten an externe Server (OpenAI, Google) gesendet?
**A:** **Nein.** Standardmäßig nutzt OpenLocalCRM eine lokale Inferenz über **Ollama (`gemma4:12b`)**, die direkt auf Ihrem Server läuft. Kein Byte an Kunden- oder Gesprächsdaten verlässt Ihre Infrastruktur.

### Q: Wie funktioniert der DSGVO-Datenschutzfilter (PII-Maskierung)?
**A:** Bevor ein Text an das Sprachmodell übergeben wird, maskiert der integrierte Prompt-Guard (`internal/ai/guard.go`) automatisch:
- **IBANs** ➔ `[REDACTED_IBAN]`
- **Kreditkartennummern** ➔ `[REDACTED_CREDIT_CARD]`
- **Passwörter & API-Keys** ➔ `[REDACTED_SECRET]`
- **Steuer-IDs** ➔ `[REDACTED_TAXID]`

### Q: Kann ich auch andere Modelle als Gemma 12B verwenden?
**A:** Ja. Über die Umgebungsvariable `OLLAMA_MODEL` können Sie jedes in Ollama verfügbare Modell definieren (z. B. `gemma4:12b-mlx`, `qwen3:8b`, `mistral:7b`).

---

## 3. D2D Feldvertrieb & Gebietskarte

### Q: Wie kommen die Adressen auf die Gebietskarte (`/map`)?
**A:** Sobald ein Kontakt oder eine Firma mit Straße, Hausnummer, PLZ und Ort angelegt wird, ermittelt das System im Hintergrund automatisch die Geokoordinaten (Latitude/Longitude) per Geocoding und platziert den Marker auf der Leaflet-Karte.

### Q: Welche Bedeutung haben die Farben der Karten-Pins?
**A:** Die Farben visualisieren direkt die **Vertriebsstufe (§7.6)**:
- 🔵 **Blau:** `Lead / Unbesucht` (Neuer Lead)
- 🟡 **Gelb:** `Setter-Gespräch` (Erstkontakt vor Ort erfolgt)
- 🟠 **Orange:** `Closer-Termin` (Verbindlicher Beratungstermin vereinbart)
- 🟢 **Grün:** `Abgeschlossen / Gewonnen` (Deal erfolgreich gezeichnet)
- ⚪ **Grau / Rot:** `Verloren` oder `Widerrufen nach § 355 BGB`

---

## 4. E-Mail, Kalender & Telefonie

### Q: Wie importiere ich Termine in Microsoft Outlook oder Apple Kalender?
**A:** Zu jedem Termin im CRM (`/calendar`) gibt es einen Button **`ICS herunterladen`**. Die generierte `.ics`-Datei entspricht dem Standard **RFC 5545** und kann mit einem Klick in Outlook, Google Kalender, Apple Kalender oder Thunderbird importiert werden.

### Q: Wie funktioniert die Click-to-Call Telefonie?
**A:** Klicks auf Telefonnummern im CRM öffnen das Anruf-Modal mit Live-Gesprächstimer. Nach dem Auflegen wählen Sie die Disposition (`Erreicht`, `Nicht erreicht`, `Mailbox`) und speichern Notizen direkt im Kontakt.

---

## 5. Automatisierung & Human-in-the-Loop (HITL)

### Q: Was bedeutet "Human-in-the-Loop" (§9.4)?
**A:** KI-Systeme im Vertrieb dürfen keine unkontrollierten E-Mails an Kunden versenden oder Deals manipulieren. Deshalb verharren alle automatisch generierten Aktionen (z. B. Angebots-Begleitmails) im Status `WAITING_APPROVAL`, bis ein Mitarbeiter den Schritt im Dashboard `/automations` explizit freigibt.

### Q: Kann ich eigene Automatisierungs-Routinen erstellen?
**A:** Ja. Im Bereich **Automatisierung & Workflows** können Admins über den Button `+ Routine anlegen` beliebige Trigger (`Neuer Lead`, `Deal WON`, `E-Mail Eingang`, `30 Tage Inaktivität`) mit mehrstufigen Folgeaktionen verknüpfen.

---

## 6. Diagnose & Fehlerbehebung (Troubleshooting)

### Problem: Port 80 / 8080 ist bereits belegt
**Symptom:** Docker meldet `bind: address already in use`.
**Lösung:**
1. Prüfen Sie, welcher Prozess den Port belegt:
   ```bash
   sudo lsof -i :80 -i :8080
   ```
2. Passen Sie in `.env` oder `docker-compose.yml` die Port-Weiterleitung an (z. B. `"8081:8080"`).

---

### Problem: Lokales Ollama antwortet nicht (`Connection refused`)
**Symptom:** KI-Anfragen schlagen fehl oder nutzen den internen Fallback.
**Lösung:**
1. Prüfen Sie, ob Ollama läuft:
   ```bash
   ollama list
   ```
2. Stellen Sie sicher, dass Ollama Anfragen von Docker-Containern akzeptiert:
   ```bash
   # Unter Linux/macOS Umgebungsvariable setzen:
   export OLLAMA_HOST=0.0.0.0:11434
   ollama serve
   ```
3. In Docker Compose nutzt der Server `http://host.docker.internal:11434`.

---

### Problem: Datenbank-Migrationen manuell wiederholen
**Symptom:** Schemaänderungen wurden nicht übernommen.
**Lösung:**
Der Server führt Migrationen beim Start automatisch aus. Sie können den Status in der Datenbank prüfen:
```bash
docker compose exec db psql -U crm_user -d crm_db -c "SELECT * FROM schema_migrations;"
```
