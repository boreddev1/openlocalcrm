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
Tabelle ist **generiert, nicht gepflegt**: Sie wird aus dem
Router-Registrierungsbaum und der Auth-Klassen-Tabelle `server.RouteAuthClasses`
(`internal/server/route_auth.go`) gelesen und darf nicht von Hand editiert werden.

```bash
UPDATE_ROUTE_INVENTORY=1 go test ./internal/server/ -run TestAPIReferenceInventoryUpdate -count=1
```

`internal/server/api_reference_test.go` vergleicht die Tabelle bei jedem
`go test`-Lauf bidirektional mit dem Router: ein Endpunkt, der im Router
existiert und hier fehlt, lässt die Tests genauso durchfallen wie ein
dokumentierter Endpunkt, den der Router nicht mehr registriert. Fehlt die
Tabelle, ist sie leer, fehlt die Spalte **Authentifizierung** oder ist eine
ihrer Zellen leer, schlägt die Prüfung fehl (fail-closed); es existiert kein
Pfad, der die Prüfung überspringt oder nur protokolliert.

Pfadparameter stehen in Chi-Schreibweise (`{id}`), genau so, wie der Router sie
registriert.

### Die Spalte `Authentifizierung` (Issue #115)

Chi-Routes melden nur Mount-Muster und HTTP-Methoden, **niemals** die
Middleware-Kette dahinter. Eine Auth-Klasse ließe sich also nicht aus dem
Router-Registrierungsbaum *ablesen* — sie muss *deklariert* werden. Genau das
leistet `server.RouteAuthClasses` in `internal/server/route_auth.go`: eine
Zeile pro Pfad-Präfix, verankert an denselben Pfad-Konstanten, die
`router.go` für die Mounts benutzt. Die Spalte ist generiert, nicht
abgeschrieben; drei Prüfungen halten sie ehrlich:

1. **Vollständigkeit:** Jede Route, die der Router registriert, muss in der
   Tabelle genau eine Klasse finden. Eine neue Route ohne deklarierte Klasse
   lässt `go test` durchfallen — niemals „unklassifiziert = okay". (Die
   breiteste Zeile `/api/v1` deklariert dabei die strenge Session-Klasse; ein
   vergessener Eintrag rutscht also nie versehentlich in „öffentlich".)
2. **Keine Leichen:** Jede Tabellenzeile muss mindestens eine registrierte
   Route abdecken; Mount entfernen, ohne die Zeile zu löschen, scheitert.
3. **Verhalten:** `internal/server/route_auth_class_test.go` schickt pro
   Route **einen** Request ohne jegliche Anmeldedaten gegen den aufgebauten
   Router (in-process, kein Netzwerk) und verlangt die zur Klasse passende
   Antwort: `401` bei `jwt+csrf` (zuerst greift `auth.AuthMiddleware`, vor
   CSRF, RBAC und Rate-Limitern), `403` bei `csrf-only` (die CSRF-Middleware
   sitzt vor allen anderen Schichten), bei `none` alles außer 401/403 — ein
   ausgelöstes Rate-Limit (`429`) lässt die Prüfung ebenfalls durchfallen,
   weil die Antwort dann keine Aussage über die Auth-Klasse trägt. Wer z. B.
   `AuthMiddleware` aus der geschützten Gruppe entfernt, fällt bei diesem
   Test durch — auch wenn die Tabelle unangetastet bliebe.

Bedeutung der Klassen:

| Klasse | Bedeutung |
| --- | --- |
| `none` | ohne Anmeldedaten erreichbar (Health, Login, Einladungsannahme). Rate-Limiting gilt trotzdem, denn Login und Invite akzeptieren erratbare Geheimnisse. |
| `csrf-only` | kein Access-Token erforderlich; `CSRFDoubleSubmitMiddleware` verlangt Deckung von CSRF-Cookie und `X-CSRF-Token`-Header (sonst 403). Die eigentliche Authentifizierung läuft über das HttpOnly-Refresh-Cookie (`SameSite=Strict`), das ein Browser cross-site nie mitsendet. |
| `jwt+csrf` | die geschützte Gruppe: Ed25519-Access-Token via `Authorization: Bearer` oder `access_token`-Cookie, inkl. `token_version`-Prüfung. Bei schreibenden Methoden verlangt die CSRF-Middleware zusätzlich Cookie/Header-Deckung — **mit einer Ausnahme**: eine Anfrage, die über ein geprüftes Bearer-Token authentifiziert wurde, ist befreit, weil sie keine Browser-Ambiente-Credentials trägt. Ob *ein bestimmter Aufrufer* also ein `X-CSRF-Token` braucht, hängt weiterhin davon ab, wie er sich authentifiziert. RBAC-Berechtigungen (`auth.RequirePermission`) setzen oberhalb dieser Klasse an, pro Mount. |
| `connector-token` | das Lead-Intake-Webhook prüft sein statisches Bearer-Token (`CONNECTOR_API_TOKEN`) im Handler selbst. |
| `jwt-inline` | der SSE-Stream prüft dasselbe Access-Token im Handler selbst (GET-only; EventSource kann keinen CSRF-Header senden, deshalb ist hier bewusst keine CSRF-Middleware montiert). |

**Grenzen des Beweises** (damit das Grün nicht mehr verspricht, als es ist):

* Der Probe beweist für `connector-token` und `jwt-inline` nur „abgelehnt ohne
  Anmeldedaten (401)" — dieselbe Beobachtung wie bei `jwt+csrf`. Ob die
  Ablehnung von `AuthMiddleware` oder vom Handler selbst kommt, entscheidet
  die deklarierte Tabellenzeile (Autorschaft direkt am Mount, dort mit
  Anker-Kommentaren), nicht der HTTP-Status.
* Die Bearer-Ausnahme der CSRF-Regel ist Eigenschaft des Aufrufers, nicht der
  Route; sie ist deshalb in der Klassenbeschreibung dokumentiert, nicht als
  eigene Spalte.
* RBAC-Berechtigungen pro Endpoint sind nicht Gegenstand der Spalte und auch
  nicht maschinell abgeleitet; `internal/server/rbac_routes_test.go` prüft
  sie verhaltensbasiert, die Beschreibungen in den Abschnitten unten bleiben
  menschengepflegt.

<!-- route-inventory:start -->
| Methode | Pfad | Authentifizierung |
| --- | --- | --- |
| POST | /api/v1/ai/chat | jwt+csrf |
| GET | /api/v1/ai/kb | jwt+csrf |
| POST | /api/v1/ai/kb | jwt+csrf |
| GET | /api/v1/ai/kb/search | jwt+csrf |
| DELETE | /api/v1/ai/kb/{id} | jwt+csrf |
| GET | /api/v1/ai/observability | jwt+csrf |
| POST | /api/v1/ai/parse-bill | jwt+csrf |
| POST | /api/v1/ai/research/company | jwt+csrf |
| GET | /api/v1/ai/research/jobs | jwt+csrf |
| POST | /api/v1/ai/research/jobs | jwt+csrf |
| POST | /api/v1/ai/synthesize-notes | jwt+csrf |
| POST | /api/v1/ai/triage | jwt+csrf |
| GET | /api/v1/appointments | jwt+csrf |
| POST | /api/v1/appointments | jwt+csrf |
| DELETE | /api/v1/appointments/{id} | jwt+csrf |
| PUT | /api/v1/appointments/{id} | jwt+csrf |
| GET | /api/v1/appointments/{id}/ics | jwt+csrf |
| GET | /api/v1/auth/accept-invite | none |
| POST | /api/v1/auth/accept-invite | none |
| POST | /api/v1/auth/change-password | jwt+csrf |
| POST | /api/v1/auth/login | none |
| POST | /api/v1/auth/logout | csrf-only |
| POST | /api/v1/auth/logout-all | jwt+csrf |
| POST | /api/v1/auth/refresh | csrf-only |
| POST | /api/v1/auth/totp/disable | jwt+csrf |
| POST | /api/v1/auth/totp/setup | jwt+csrf |
| POST | /api/v1/auth/totp/verify | jwt+csrf |
| GET | /api/v1/automations | jwt+csrf |
| POST | /api/v1/automations | jwt+csrf |
| GET | /api/v1/automations/runs | jwt+csrf |
| POST | /api/v1/automations/runs/{id}/approve | jwt+csrf |
| POST | /api/v1/automations/{id}/test-run | jwt+csrf |
| POST | /api/v1/backup/drill | jwt+csrf |
| GET | /api/v1/backup/export | jwt+csrf |
| POST | /api/v1/backup/restore | jwt+csrf |
| POST | /api/v1/calculator/solar | jwt+csrf |
| GET | /api/v1/companies | jwt+csrf |
| POST | /api/v1/companies | jwt+csrf |
| DELETE | /api/v1/companies/{id} | jwt+csrf |
| GET | /api/v1/companies/{id} | jwt+csrf |
| PUT | /api/v1/companies/{id} | jwt+csrf |
| POST | /api/v1/connectors/lead-intake | connector-token |
| GET | /api/v1/contacts | jwt+csrf |
| POST | /api/v1/contacts | jwt+csrf |
| DELETE | /api/v1/contacts/{id} | jwt+csrf |
| GET | /api/v1/contacts/{id} | jwt+csrf |
| PUT | /api/v1/contacts/{id} | jwt+csrf |
| GET | /api/v1/deals | jwt+csrf |
| POST | /api/v1/deals | jwt+csrf |
| DELETE | /api/v1/deals/{id} | jwt+csrf |
| GET | /api/v1/deals/{id} | jwt+csrf |
| PUT | /api/v1/deals/{id} | jwt+csrf |
| POST | /api/v1/deals/{id}/revoke | jwt+csrf |
| POST | /api/v1/deals/{id}/solar-calculation | jwt+csrf |
| GET | /api/v1/email-templates | jwt+csrf |
| POST | /api/v1/email-templates | jwt+csrf |
| POST | /api/v1/email-templates/generate | jwt+csrf |
| DELETE | /api/v1/email-templates/{key} | jwt+csrf |
| PUT | /api/v1/email-templates/{key} | jwt+csrf |
| GET | /api/v1/emails/accounts | jwt+csrf |
| POST | /api/v1/emails/accounts | jwt+csrf |
| DELETE | /api/v1/emails/accounts/{id} | jwt+csrf |
| PATCH | /api/v1/emails/accounts/{id} | jwt+csrf |
| POST | /api/v1/emails/accounts/{id}/sync | jwt+csrf |
| POST | /api/v1/emails/accounts/{id}/test | jwt+csrf |
| POST | /api/v1/emails/demo-ingest | jwt+csrf |
| GET | /api/v1/emails/messages | jwt+csrf |
| POST | /api/v1/emails/send | jwt+csrf |
| GET | /api/v1/emails/threads/{threadID} | jwt+csrf |
| PATCH | /api/v1/emails/{id}/tags | jwt+csrf |
| GET | /api/v1/export/contacts.csv | jwt+csrf |
| GET | /api/v1/health | none |
| POST | /api/v1/import/contacts | jwt+csrf |
| GET | /api/v1/me | jwt+csrf |
| GET | /api/v1/notes | jwt+csrf |
| POST | /api/v1/notes | jwt+csrf |
| DELETE | /api/v1/notes/{id} | jwt+csrf |
| GET | /api/v1/notes/{id} | jwt+csrf |
| PUT | /api/v1/notes/{id} | jwt+csrf |
| POST | /api/v1/notifications/read-all | jwt+csrf |
| GET | /api/v1/notifications/unread | jwt+csrf |
| POST | /api/v1/notifications/{id}/read | jwt+csrf |
| POST | /api/v1/ownership/bulk-assign | jwt+csrf |
| GET | /api/v1/reports/sales | jwt+csrf |
| GET | /api/v1/settings/export | jwt+csrf |
| POST | /api/v1/settings/import | jwt+csrf |
| POST | /api/v1/telephony/calls | jwt+csrf |
| GET | /api/v1/todos | jwt+csrf |
| POST | /api/v1/todos | jwt+csrf |
| DELETE | /api/v1/todos/{id} | jwt+csrf |
| GET | /api/v1/todos/{id} | jwt+csrf |
| PUT | /api/v1/todos/{id} | jwt+csrf |
| GET | /api/v1/users | jwt+csrf |
| POST | /api/v1/users/invite | jwt+csrf |
| PUT | /api/v1/users/{id}/role | jwt+csrf |
| PUT | /api/v1/users/{id}/status | jwt+csrf |
| GET | /events/stream | jwt-inline |
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
