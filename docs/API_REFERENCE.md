# 📡 REST API & Webhook Referenz

Vollständige technische Dokumentation aller REST-Endpunkte, Authentifizierungsmechanismen und Webhooks von **OpenLocalCRM (Single-Tenant v3)**.

---

## 🔐 Authentifizierung

Die API unterstützt zwei Authentifizierungsmodi:

1. **User JWT (Ed25519 Signatur):** Für Benutzer und das Single-Page-Application Frontend (wird als HttpOnly Cookie oder Bearer Header übertragen).
   ```http
   Authorization: Bearer <jwt_access_token>
   ```

2. **Connector API Token:** Für externe Webhooks (Lead-Intake, Website-Formulare, Tarifrechner).
   ```http
   Authorization: Bearer <connector_api_token>
   ```

---

## 📋 Endpunkt-Übersicht

### 1. Healthcheck & Systemstatus

```bash
curl -X GET http://localhost:8080/api/v1/health
```

#### Response (`200 OK`)
```json
{
  "status": "healthy",
  "system": "openlocalcrm-v3",
  "version": "3.0.0",
  "demo_mode": false
}
```

---

### 2. Authentifizierung (`/api/v1/auth`)

Alle Authentifizierungs-Endpunkte sind durch einen Sliding-Window Rate-Limiter (max. 5 Fehlversuche / 5 Minuten) geschützt.

#### Anmelden (Login)
Überprüft die Anmeldedaten mit Argon2id und liefert einen signierten Ed25519-JWT sowie ein sicheres HttpOnly-Cookie `access_token`. Falls TOTP für das Konto aktiviert ist, muss `totp_code` übergeben werden.

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@openlocalcrm.local",
    "password": "IhrSicheresPasswort123!",
    "totp_code": "123456"
  }'
```

##### Response (`200 OK`)
```json
{
  "token": "eyJhbGciOiJFZERTQSI...",
  "user": {
    "user_id": "00000000-0000-0000-0000-000000000001",
    "email": "admin@openlocalcrm.local",
    "role": "ADMIN",
    "first_name": "Max",
    "last_name": "Administrator",
    "totp_enabled": true
  }
}
```

*Hinweis:* Falls 2FA aktiv ist und kein `totp_code` übergeben wird, antwortet die API mit `401 Unauthorized` und `{ "error": "TOTP code required", "code": "TOTP_REQUIRED" }`.

#### Token erneuern (`/refresh`)
Verlängert die Sitzung und gibt ein frisches JWT-Token aus.

```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Authorization: Bearer <token>"
```

#### Passwort ändern (`/change-password`)
Aktualisiert das Passwort mittels Argon2id Hash nach Prüfung des aktuellen Passworts.

```bash
curl -X POST http://localhost:8080/api/v1/auth/change-password \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "current_password": "AltesPasswort123!",
    "new_password": "NeuesSicheresPasswort456!"
  }'
```

#### 2FA / TOTP Setup einleiten (`/totp/setup`)
Erzeugt ein neues RFC 6238 Secret und liefert den Secret-Key sowie die `otpauth://`-URI für QR-Codes.

```bash
curl -X POST http://localhost:8080/api/v1/auth/totp/setup \
  -H "Authorization: Bearer <token>"
```

##### Response (`200 OK`)
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "qr_uri": "otpauth://totp/OpenLocalCRM:admin@openlocalcrm.local?secret=JBSWY3DPEHPK3PXP&issuer=OpenLocalCRM"
}
```

#### 2FA / TOTP aktivieren (`/totp/verify`)
Verifiziert das Secret anhand des 6-stelligen Codes aus der Authenticator-App und schaltet 2FA scharf.

```bash
curl -X POST http://localhost:8080/api/v1/auth/totp/verify \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "123456"
  }'
```

#### 2FA / TOTP deaktivieren (`/totp/disable`)
Deaktiviert 2FA nach Bestätigung des aktuellen Kontokennworts.

```bash
curl -X POST http://localhost:8080/api/v1/auth/totp/disable \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "IhrSicheresPasswort123!"
  }'
```

#### Eigene Session abrufen (`/me`)
```bash
curl -X GET http://localhost:8080/api/v1/me \
  -H "Authorization: Bearer <token>"
```

#### Abmelden (Logout)
```bash
curl -X POST http://localhost:8080/api/v1/auth/logout
```

---

### 3. Lead-Intake Webhook (Website-Formulare & Portale)

Nimmt externe Leads entgegen, führt Deduplizierung durch und legt automatisch Kontakte/Deals an.

```bash
curl -X POST http://localhost:8080/api/v1/connectors/lead-intake \
  -H "Authorization: Bearer openlocalcrm-secret-connector-token" \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Dr. Michael",
    "last_name": "Weber",
    "email": "weber@energie-dach.de",
    "phone": "+49 69 12345678",
    "company_name": "Energie Südwest GmbH",
    "deal_title": "30 kWp Gewerbedach Solaranlage",
    "deal_amount": 38500.0,
    "consent_phone": true,
    "consent_email": true
  }'
```

#### Response (`201 Created`)
```json
{
  "status": "created",
  "contact_id": "c-1724541234",
  "deal_id": "d-1724541235",
  "source": "WEB_FORM_INTAKE"
}
```

---

### 4. Kontakte & Leads (`/api/v1/contacts`)

#### Kontakte auflisten / suchen
```bash
curl -X GET "http://localhost:8080/api/v1/contacts?q=Weber&limit=20&offset=0" \
  -H "Authorization: Bearer <token>"
```

#### Kontakt erstellen
```bash
curl -X POST http://localhost:8080/api/v1/contacts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Sabine",
    "last_name": "Mustermann",
    "email": "sabine@mustermann.de",
    "phone": "+49 171 9876543",
    "address_street": "Goethestraße 8",
    "address_zip": "60313",
    "address_city": "Frankfurt am Main",
    "lead_source": "D2D_FIELD"
  }'
```

#### Kontakt per ID abrufen
```bash
curl -X GET http://localhost:8080/api/v1/contacts/<contact_id> \
  -H "Authorization: Bearer <token>"
```

#### Kontakt aktualisieren
```bash
curl -X PUT http://localhost:8080/api/v1/contacts/<contact_id> \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Sabine",
    "last_name": "Mustermann",
    "email": "sabine.neu@mustermann.de"
  }'
```

#### Kontakt löschen
```bash
curl -X DELETE http://localhost:8080/api/v1/contacts/<contact_id> \
  -H "Authorization: Bearer <token>"
```

---

### 5. Deals & Pipeline (`/api/v1/deals`)

#### Deals abrufen (optional nach Vertriebsphase)
```bash
curl -X GET "http://localhost:8080/api/v1/deals?stage=OFFER_SENT" \
  -H "Authorization: Bearer <token>"
```

#### Deal erstellen
```bash
curl -X POST http://localhost:8080/api/v1/deals \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "30 kWp Gewerbedach Solaranlage Weber",
    "stage": "LEAD",
    "currency": "EUR",
    "probability": 20
  }'
```

#### Deal aktualisieren
```bash
curl -X PUT http://localhost:8080/api/v1/deals/<deal_id> \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "stage": "OFFER_SENT",
    "probability": 60,
    "value": 38500.00
  }'
```

#### Deal löschen
```bash
curl -X DELETE http://localhost:8080/api/v1/deals/<deal_id> \
  -H "Authorization: Bearer <token>"
```

---

### 6. Notizen & KI-Synthese (`/api/v1/notes` & `/api/v1/ai`)

#### Notizen zu Kontakt/Deal abrufen
```bash
curl -X GET "http://localhost:8080/api/v1/notes?entity_type=contact&entity_id=<contact_id>" \
  -H "Authorization: Bearer <token>"
```

#### Neue Notiz erstellen
```bash
curl -X POST http://localhost:8080/api/v1/notes \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "entity_type": "contact",
    "entity_id": "<contact_id>",
    "type": "CALL",
    "author": "Max Vertriebsleiter",
    "content": "Kunde hat Interesse an 20 kWh Batteriespeicher. Statik liegt vor."
  }'
```

#### KI-Notizen-Synthese (Zusammenfassung & Handlungskarten)
```bash
curl -X POST http://localhost:8080/api/v1/ai/synthesize-notes \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "contact_id": "<contact_id>"
  }'
```

---

### 7. Termine & RFC 5545 ICS-Export (`/api/v1/appointments`)

#### Termin erstellen
```bash
curl -X POST http://localhost:8080/api/v1/appointments \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Vor-Ort Begehung PV 25 kWp",
    "start_time": "2026-08-26T10:00:00Z",
    "end_time": "2026-08-26T11:30:00Z",
    "location": "Kaiserstraße 14, 60311 Frankfurt am Main",
    "notes": "Dachzustand und Zählerkasten begutachten"
  }'
```

#### RFC 5545 `.ics` Kalendereinladung herunterladen
```bash
curl -X GET http://localhost:8080/api/v1/appointments/app-1/ics \
  -H "Authorization: Bearer <token>" \
  -o termin.ics
```

---

### 6. Click-to-Call Telefonie

```bash
curl -X POST http://localhost:8080/api/v1/telephony/calls \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "contact_id": "c-1",
    "duration_seconds": 184,
    "disposition": "REACHED",
    "notes": "Kunde bestätigt Termin am Donnerstag."
  }'
```

---

### 7. Automatisierungs-Engine & Workflows

#### Aktive Workflows auflisten
```bash
curl -X GET http://localhost:8080/api/v1/automations \
  -H "Authorization: Bearer <token>"
```

#### Workflow-Läufe & HITL-Freigaben
```bash
curl -X GET http://localhost:8080/api/v1/automations/runs \
  -H "Authorization: Bearer <token>"
```

---

### 8. KI-Gateway (Gemma 12B & PII-Filter)

#### E-Mail Triage & Entwurf
```bash
curl -X POST http://localhost:8080/api/v1/ai/triage \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "sender": "kunde@solar-dach.de",
    "subject": "Anfrage 20 kWp PV-Anlage",
    "body": "Hallo, bitte erstellen Sie ein Angebot. Meine IBAN ist DE89370400440532013000."
  }'
```

#### Response (`200 OK` — PII automatisch maskiert)
```json
{
  "category": "ANFRAGE",
  "sentiment": "POSITIVE",
  "priority": "HIGH",
  "summary": "Kunde bittet um Angebot für eine 20 kWp Photovoltaikanlage.",
  "draft_reply": "Sehr geehrte Damen und Herren,\n\nvielen Dank für Ihre Anfrage. Gerne erstellen wir Ihnen ein detailliertes Angebot.\n\nMit freundlichen Grüßen,\nIhr Vertriebsteam"
}
```

#### EU AI Act Observability-Metriken
```bash
curl -X GET http://localhost:8080/api/v1/ai/observability \
  -H "Authorization: Bearer <token>"
```

---

### 9. Server-Sent Events (SSE Realtime Stream)

Der SSE-Stream liefert Systemereignisse in Echtzeit. Aus Sicherheitsgründen erfordert dieser Endpunkt eine gültige Authentifizierung über ein Bearer-Token, einen Query-Parameter (`?token=<jwt>`) oder das `access_token`-Cookie.

```bash
curl -N -H "Accept: text/event-stream" \
  -H "Authorization: Bearer <token>" \
  http://localhost:8080/events/stream
```

#### Event-Stream Format:
```text
event: connected
data: {"status":"connected"}

event: appointment.created
data: {"id":"app-102","title":"Vor-Ort Begehung"}

event: email.received
data: {"id":"m-99","subject":"Neues PV-Projekt"}
```

---

### 10. Team- & Benutzerverwaltung (`/api/v1/users`)

*Hinweis: Alle Benutzerverwaltungs-Endpunkte erfordern die Rolle `ADMIN`.*

#### Benutzerliste abrufen
```bash
curl -X GET http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

##### Response (`200 OK`)
```json
[
  {
    "id": "00000000-0000-0000-0000-000000000001",
    "email": "admin@openlocalcrm.local",
    "name": "Max Administrator",
    "role": "ADMIN",
    "status": "ACTIVE",
    "totp_enabled": true,
    "created_at": "2026-08-25T10:00:00Z"
  }
]
```

#### Neues Teammitglied einladen
Erstellt ein neues Benutzerkonto mit sicherem Zufallspasswort oder Einladungs-Token.

```bash
curl -X POST http://localhost:8080/api/v1/users/invite \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "kollege@openlocalcrm.local",
    "name": "Sarah Vertrieb",
    "role": "VERTRIEB"
  }'
```

#### Benutzerrolle ändern (mit Last-Admin-Schutz)
Verhindert, dass dem letzten aktiven Administrator die Administrator-Rechte entzogen werden.

```bash
curl -X PUT http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000002/role \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "role": "BACKOFFICE"
  }'
```

#### Benutzerstatus ändern (Aktivieren / Deaktivieren)
Verhindert die Deaktivierung des letzten aktiven Administrators.

```bash
curl -X PUT http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000002/status \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "SUSPENDED"
  }'
```

---

### 11. Backup & Revisionssicherheit (`/api/v1/backup`)

*Hinweis: Alle Backup-Endpunkte erfordern die Rolle `ADMIN`.*

#### Restore-Drill & Konsistenzprüfung (`/drill`)
Überprüft die Verbindung zur Datenbank, zählt die Datensätze aller Kern-Tabellen (Kontakte, Deals, Aufgaben, Notizen, Audits) und meldet den Konsistenzstatus.

```bash
curl -X POST http://localhost:8080/api/v1/backup/drill \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

##### Response (`200 OK`)
```json
{
  "status": "HEALTHY",
  "checked_at": "2026-09-14T09:00:00Z",
  "tables": {
    "users": 2,
    "contacts": 142,
    "deals": 38,
    "todos": 19,
    "audit_logs": 512
  }
}
```

#### Vollständiger CRM JSON-Export (`/export`)
Erstellt einen revisionssicheren JSON-Dump des gesamten Mandanten für Offsite-Sicherungen oder Migrationen.

```bash
curl -X GET http://localhost:8080/api/v1/backup/export \
  -H "Authorization: Bearer <ADMIN_TOKEN>" > openlocalcrm_backup.json
```
