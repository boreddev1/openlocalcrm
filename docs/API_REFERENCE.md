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

### Routen-Inventur (maschinell erzeugt)

Vollständige Liste aller Endpunkte, die `internal/server/router.go` registriert —
die Gruppe `/api/v1/…` und der SSE-Stream `/events/stream`. Der eingebettete SPA-
Mount (`r.Mount("/", web.DistHandler())`) ist bewusst ausgenommen: er bedient
Frontend-Dateien statt API-Endpunkte und hat keine abgrenzbare Pfadliste. Die
Tabelle ist **generiert, nicht gepflegt**: Sie wird aus dem laufenden Router
gelesen und darf nicht von Hand editiert werden.

```bash
UPDATE_ROUTE_INVENTORY=1 go test ./internal/server/ -run TestAPIReferenceInventoryUpdate -count=1
```

`internal/server/api_reference_test.go` vergleicht die Tabelle bei jedem
`go test`-Lauf bidirektional mit dem Router: ein Endpunkt, der im Router
existiert und hier fehlt, lässt die Tests genauso durchfallen wie ein
dokumentierter Endpunkt, den der Router nicht mehr registriert. Fehlt die
Tabelle oder ist sie leer, schlägt die Prüfung fehl (fail-closed); es existiert
kein Pfad, der die Prüfung überspringt oder nur protokolliert.

Pfadparameter stehen in Chi-Schreibweise (`{id}`), genau so, wie der Router sie
registriert. Die Tabelle nennt bewusst **keine** Auth-Spalte: eine
HTTP-Methode-Pfad-Kombination ist alles, was sich maschinell aus dem Router
ablesen lässt. Ob ein Endpunkt ein `X-CSRF-Token` verlangt, hängt nicht an der
Route, sondern daran, wie sich der Aufrufer authentifiziert (die CSRF-Middleware
befreit eine per Bearer-Token geprüfte Anfrage), und lässt sich aus Chi-Routes
nicht auslesen — die Regeln stehen deshalb in den Abschnitten unten.

<!-- route-inventory:start -->
| Methode | Pfad |
| --- | --- |
| POST | /api/v1/ai/chat |
| GET | /api/v1/ai/kb |
| POST | /api/v1/ai/kb |
| GET | /api/v1/ai/kb/search |
| DELETE | /api/v1/ai/kb/{id} |
| GET | /api/v1/ai/observability |
| POST | /api/v1/ai/parse-bill |
| POST | /api/v1/ai/research/company |
| GET | /api/v1/ai/research/jobs |
| POST | /api/v1/ai/research/jobs |
| POST | /api/v1/ai/synthesize-notes |
| POST | /api/v1/ai/triage |
| GET | /api/v1/appointments |
| POST | /api/v1/appointments |
| DELETE | /api/v1/appointments/{id} |
| PUT | /api/v1/appointments/{id} |
| GET | /api/v1/appointments/{id}/ics |
| GET | /api/v1/auth/accept-invite |
| POST | /api/v1/auth/accept-invite |
| POST | /api/v1/auth/change-password |
| POST | /api/v1/auth/login |
| POST | /api/v1/auth/logout |
| POST | /api/v1/auth/logout-all |
| POST | /api/v1/auth/refresh |
| POST | /api/v1/auth/totp/disable |
| POST | /api/v1/auth/totp/setup |
| POST | /api/v1/auth/totp/verify |
| GET | /api/v1/automations |
| POST | /api/v1/automations |
| GET | /api/v1/automations/runs |
| POST | /api/v1/automations/runs/{id}/approve |
| POST | /api/v1/automations/{id}/test-run |
| POST | /api/v1/backup/drill |
| GET | /api/v1/backup/export |
| POST | /api/v1/backup/restore |
| POST | /api/v1/calculator/solar |
| GET | /api/v1/companies |
| POST | /api/v1/companies |
| DELETE | /api/v1/companies/{id} |
| GET | /api/v1/companies/{id} |
| PUT | /api/v1/companies/{id} |
| POST | /api/v1/connectors/lead-intake |
| GET | /api/v1/contacts |
| POST | /api/v1/contacts |
| DELETE | /api/v1/contacts/{id} |
| GET | /api/v1/contacts/{id} |
| PUT | /api/v1/contacts/{id} |
| GET | /api/v1/deals |
| POST | /api/v1/deals |
| DELETE | /api/v1/deals/{id} |
| GET | /api/v1/deals/{id} |
| PUT | /api/v1/deals/{id} |
| POST | /api/v1/deals/{id}/revoke |
| POST | /api/v1/deals/{id}/solar-calculation |
| GET | /api/v1/email-templates |
| POST | /api/v1/email-templates |
| POST | /api/v1/email-templates/generate |
| DELETE | /api/v1/email-templates/{key} |
| PUT | /api/v1/email-templates/{key} |
| GET | /api/v1/emails/accounts |
| POST | /api/v1/emails/accounts |
| DELETE | /api/v1/emails/accounts/{id} |
| PATCH | /api/v1/emails/accounts/{id} |
| POST | /api/v1/emails/accounts/{id}/sync |
| POST | /api/v1/emails/accounts/{id}/test |
| POST | /api/v1/emails/demo-ingest |
| GET | /api/v1/emails/messages |
| POST | /api/v1/emails/send |
| GET | /api/v1/emails/threads/{threadID} |
| PATCH | /api/v1/emails/{id}/tags |
| GET | /api/v1/export/contacts.csv |
| GET | /api/v1/health |
| POST | /api/v1/import/contacts |
| GET | /api/v1/me |
| GET | /api/v1/notes |
| POST | /api/v1/notes |
| DELETE | /api/v1/notes/{id} |
| GET | /api/v1/notes/{id} |
| PUT | /api/v1/notes/{id} |
| POST | /api/v1/notifications/read-all |
| GET | /api/v1/notifications/unread |
| POST | /api/v1/notifications/{id}/read |
| POST | /api/v1/ownership/bulk-assign |
| GET | /api/v1/reports/sales |
| GET | /api/v1/settings/export |
| POST | /api/v1/settings/import |
| POST | /api/v1/telephony/calls |
| GET | /api/v1/todos |
| POST | /api/v1/todos |
| DELETE | /api/v1/todos/{id} |
| GET | /api/v1/todos/{id} |
| PUT | /api/v1/todos/{id} |
| GET | /api/v1/users |
| POST | /api/v1/users/invite |
| PUT | /api/v1/users/{id}/role |
| PUT | /api/v1/users/{id}/status |
| GET | /events/stream |
<!-- route-inventory:end -->

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
    "totp_enabled": true,
    "must_change_password": false
  }
}
```

*Hinweis:* Falls 2FA aktiv ist und kein `totp_code` übergeben wird, antwortet die API mit `401 Unauthorized` und `{ "error": "TOTP code required", "code": "TOTP_REQUIRED" }`.

#### Token erneuern (`/refresh`)
Verlängert die Sitzung via Single-Use Refresh Token Rotation. Aus Sicherheitsgründen wird der Refresh-Token ausschließlich über das HttpOnly-Cookie `refresh_token` übergeben (kein Bearer-Header). Erzeugt ein neues Access-Token und rotiert den Refresh-Token.

```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  --cookie "refresh_token=<token>"
```

#### Alle Sitzungen abmelden (`/auth/logout-all`)
Meldet alle aktiven Sitzungen des Benutzers auf allen Geräten sofort ab, indem die `token_version` des Benutzers inkrementiert und alle Refresh-Tokens der Token-Familie revidiert werden.

```bash
curl -X POST http://localhost:8080/api/v1/auth/logout-all \
  -H "Authorization: Bearer <token>" \
  -H "X-CSRF-Token: <csrf_token>"
```

#### Passwort ändern (`/change-password`)
Aktualisiert das Passwort mittels Argon2id Hash nach Prüfung des aktuellen Passworts (Mindestlänge 8 Zeichen). Setzt `must_change_password` auf `false` und invalidiert bestehende Sessions durch Hochzählen der `token_version`.

```bash
curl -X POST http://localhost:8080/api/v1/auth/change-password \
  -H "Authorization: Bearer <token>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "old_password": "AltesPasswort123!",
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
  "url": "otpauth://totp/OpenLocalCRM:admin@openlocalcrm.local?secret=JBSWY3DPEHPK3PXP&issuer=OpenLocalCRM"
}
```

#### 2FA / TOTP aktivieren (`/totp/verify`)
Verifiziert das Secret anhand des 6-stelligen Codes aus der Authenticator-App und schaltet 2FA scharf.

```bash
curl -X POST http://localhost:8080/api/v1/auth/totp/verify \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "123456",
    "secret": "JBSWY3DPEHPK3PXP"
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

##### Response (`200 OK`)
```json
{
  "user_id": "00000000-0000-0000-0000-000000000001",
  "email": "admin@openlocalcrm.local",
  "role": "ADMIN",
  "first_name": "Max",
  "last_name": "Administrator",
  "totp_enabled": false,
  "must_change_password": false
}
```

---

#### Abmelden (Logout)
```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Authorization: Bearer <token>" \
  -H "X-CSRF-Token: <csrf_token>"
```

---

### 3. Datensatz-Eigentümerschaft & Neuzuweisung (`/api/v1/ownership`)

Administrativer Endpunkt zur Massen-Neuzuweisung von CRM-Datensätzen (Kontakte, Firmen, Deals, Termine, Aufgaben, Notizen) von einem Vorbesitzer auf einen neuen Eigentümer.

```bash
curl -X POST http://localhost:8080/api/v1/ownership/bulk-assign \
  -H "Authorization: Bearer <admin_token>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "from_user_id": "11111111-1111-1111-1111-111111111111",
    "to_user_id": "22222222-2222-2222-2222-222222222222",
    "entity_types": ["contacts", "companies", "deals", "appointments", "todos", "notes"]
  }'
```

---

### 4. Lead-Intake Webhook (Website-Formulare & Portale)

Nimmt externe Leads entgegen, führt Deduplizierung durch und legt automatisch Kontakte/Deals an.

```bash
curl -X POST http://localhost:8080/api/v1/connectors/lead-intake \
  -H "Authorization: Bearer demo-connector-token" \
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
  "contact_id": "00000000-0000-0000-0000-000000000001",
  "deal_id": "00000000-0000-0000-0000-000000000002",
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

### 8. Click-to-Call Telefonie (`/api/v1/telephony`)

```bash
curl -X POST http://localhost:8080/api/v1/telephony/calls \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "contact_id": "00000000-0000-0000-0000-000000000001",
    "duration_seconds": 184,
    "disposition": "REACHED",
    "notes": "Kunde bestätigt Termin am Donnerstag."
  }'
```

---

### 9. Automatisierungs-Engine & Workflows (`/api/v1/automations`)

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

### 10. KI-Gateway (Gemma 12B & PII-Filter) (`/api/v1/ai`)

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

### 11. Server-Sent Events (SSE Realtime Stream) (`/events/stream`)

Der SSE-Stream liefert Systemereignisse in Echtzeit über ein aktives `access_token`-Cookie oder den `Authorization: Bearer`-Header.

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

### 12. Team- & Benutzerverwaltung (`/api/v1/users`)

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
Erstellt ein neues Benutzerkonto mit Rolle (`ADMIN`, `BENUTZER`, `VERTRIEB`, `BACKOFFICE`).

```bash
curl -X POST http://localhost:8080/api/v1/users/invite \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "sarah.vertrieb@openlocalcrm.local",
    "name": "Sarah Sommer",
    "role": "VERTRIEB"
  }'
```

#### Rolle ändern
```bash
curl -X PUT http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000002/role \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "role": "BACKOFFICE"
  }'
```

#### Benutzer sperren / deaktivieren
```bash
curl -X PUT http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000002/status \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "DEACTIVATED"
  }'
```

---

### 13. Backup & Revisionssicherheit (`/api/v1/backup`)

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

---

### 14. Firmen, Aufgaben & Benachrichtigungen (`/api/v1/companies`, `/api/v1/todos`, `/api/v1/notifications`)

#### Firmen auflisten & anlegen
```bash
curl -X GET http://localhost:8080/api/v1/companies \
  -H "Authorization: Bearer <token>"
```

#### Aufgaben verwalten
```bash
curl -X GET http://localhost:8080/api/v1/todos \
  -H "Authorization: Bearer <token>"
```

#### Ungelesene Benachrichtigungen abrufen
```bash
curl -X GET http://localhost:8080/api/v1/notifications/unread \
  -H "Authorization: Bearer <token>"
```
