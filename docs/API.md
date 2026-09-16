# 📡 REST API & Webhook Referenz

Alle REST-Endpunkte sind unter `/api/v1` gemountet.

---

## 1. Authentifizierung

| Header | Wert | Beschreibung |
| :--- | :--- | :--- |
| `Authorization` | `Bearer <Ed25519_JWT_Token>` | Für Benutzerinteraktionen (oder via `HttpOnly`-Cookie `access_token`) |
| `Authorization` | `Bearer <Connector_API_Token>` | Für Konnektoren & Webhooks (`/api/v1/connectors/lead-intake`) |

---

## 2. Endpunkte

### Healthcheck
```http
GET /api/v1/health
```
**Response (200 OK):**
```json
{
  "status": "healthy",
  "system": "openlocalcrm-v3",
  "version": "3.0.0",
  "demo_mode": false
}
```

---

### Authentifizierung (`/api/v1/auth`)

#### Anmelden
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "admin@openlocalcrm.local",
  "password": "IhrPasswort123!",
  "totp_code": "123456"
}
```

#### Token erneuern
```http
POST /api/v1/auth/refresh
Authorization: Bearer <Ed25519_JWT_Token>
```

#### Passwort ändern
```http
POST /api/v1/auth/change-password
Authorization: Bearer <Ed25519_JWT_Token>
Content-Type: application/json

{
  "current_password": "AltesPasswort123!",
  "new_password": "NeuesSicheresPasswort456!"
}
```

#### 2FA / TOTP Setup
```http
POST /api/v1/auth/totp/setup
Authorization: Bearer <Ed25519_JWT_Token>
```

#### 2FA / TOTP Verifizieren
```http
POST /api/v1/auth/totp/verify
Authorization: Bearer <Ed25519_JWT_Token>
Content-Type: application/json

{
  "code": "123456"
}
```

#### Eigene Benutzerdaten abrufen
```http
GET /api/v1/me
Authorization: Bearer <Ed25519_JWT_Token>
```

#### Abmelden
```http
POST /api/v1/auth/logout
```

---

### Kontakte & Leads (`/api/v1/contacts`)

#### Kontakte auflisten / suchen
```http
GET /api/v1/contacts?q=Muster&limit=20&offset=0
```

#### Neuen Kontakt anlegen
```http
POST /api/v1/contacts
Content-Type: application/json

{
  "first_name": "Erika",
  "last_name": "Musterfrau",
  "email": "erika@energie-kunden.de",
  "phone": "+49 69 98765432",
  "address_street": "Mainzer Landstraße 45",
  "address_zip": "60329",
  "address_city": "Frankfurt am Main",
  "zaehlernummer": "1EMH123456789",
  "consent_phone": true,
  "consent_email": true
}
```

---

### Deal-Pipeline (`/api/v1/deals`)

#### Deals nach Phase abrufen
```http
GET /api/v1/deals?stage=LEAD
```

#### Deal anlegen
```http
POST /api/v1/deals
Content-Type: application/json

{
  "title": "15 kWp PV-Anlage & 10kWh Speicher",
  "value": "21500.00",
  "currency": "EUR",
  "stage": "LEAD",
  "probability": 20
}
```

---

### Lead-Intake Webhook (`/api/v1/connectors/lead-intake`)

```http
POST /api/v1/connectors/lead-intake
Authorization: Bearer openlocalcrm-secret-connector-token
Content-Type: application/json

{
  "first_name": "Sabine",
  "last_name": "Mustermann",
  "email": "sabine.mustermann@web.de",
  "phone": "+49 69 12345678",
  "street": "Zeil 100",
  "city": "Frankfurt am Main",
  "zip": "60313",
  "deal_title": "PV-Anlage 12 kWp + Speicher",
  "deal_value": "18500.00",
  "source": "WEBSITE_CALCULATOR"
}
```

---

### KI-Triage (`/api/v1/ai/triage`)

```http
POST /api/v1/ai/triage
Content-Type: application/json

{
  "sender": "kunde@solar-test.de",
  "subject": "Anfrage Solaranlage mit Speicher",
  "body": "Guten Tag, wir interessieren uns für eine 10 kWp PV-Anlage. Bitte Angebot senden."
}
```
**Response (200 OK):**
```json
{
  "category": "ANFRAGE",
  "sentiment": "POSITIVE",
  "priority": "HIGH",
  "summary": "Kunde interessiert sich für PV-Anlage & Speicher und bittet um Angebot.",
  "draft_reply": "Sehr geehrte Damen und Herren,\n\nvielen Dank für Ihre Anfrage..."
}
```

---

### Echtzeit-Events (`/events/stream`)

```http
GET /events/stream
Authorization: Bearer <Ed25519_JWT_Token>
Accept: text/event-stream
```

---

### Benutzerverwaltung (`/api/v1/users` — Admin)

```http
GET /api/v1/users
Authorization: Bearer <ADMIN_JWT_Token>
```

```http
POST /api/v1/users/invite
Authorization: Bearer <ADMIN_JWT_Token>
Content-Type: application/json

{
  "email": "vertrieb@openlocalcrm.local",
  "name": "Alex Vertrieb",
  "role": "VERTRIEB"
}
```

```http
PUT /api/v1/users/{id}/role
Authorization: Bearer <ADMIN_JWT_Token>
Content-Type: application/json

{
  "role": "BACKOFFICE"
}
```

---

### Backup & Konsistenz (`/api/v1/backup` — Admin)

```http
POST /api/v1/backup/drill
Authorization: Bearer <ADMIN_JWT_Token>
```

```http
GET /api/v1/backup/export
Authorization: Bearer <ADMIN_JWT_Token>
```
