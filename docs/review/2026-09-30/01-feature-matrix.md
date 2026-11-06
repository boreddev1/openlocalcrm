# 01 – Feature-Vollständigkeits-Matrix (Review 2026-09-30)

Stand: Commit c6bac3e (main, Arbeitsbaum sauber). Read-only Review, Task 1 des Plans `docs/superpowers/plans/2026-09-30-full-source-review-plan.md`.
Methode: Routen aus `internal/server/router.go` (Z. 135-627), Kette UI (`web/src/pages`, `web/src/api/client.ts`) -> Route -> Handler -> `internal/core/<d>` -> `internal/db/queries/*.sql` per Code-Lektüre nachverfolgt; Doku gegen `docs/API_REFERENCE.md` abgeglichen; E2E gegen `e2e/*.spec.ts` (18 Dateien, 1923 Zeilen) gegrept.
Legende: `ja` = vorhanden und belegt, `teil` = vorhanden mit Lücke (siehe Findings), `nein` = fehlt. Unit-Test = `*_test.go` für Core bzw. Handler; E2E = Playwright-Spec mit Bezug zur Domäne.

## 1. Matrix

| Domäne | Doku | Route | Handler | Core | SQL | UI | Unit-Test | E2E | Status |
|---|---|---|---|---|---|---|---|---|---|
| Kontakte (CRUD, Suche, Einwilligung, Import-Felder) | ja (`API_REFERENCE.md:231-280`, ohne Liste-Filter/Import) | ja `router.go:459-465` | ja `handlers/contacts.go` | ja `core/contact` | ja `queries/contacts.sql` | ja `ContactsPage.tsx`, `CSVImportModal.tsx` | ja (Core 68 %, Handler, Ownership) | ja `contacts.spec.ts` | voll, aber Einwilligung nicht durchgesetzt (F-1-03), Löschung unvollständig (F-1-04) |
| Firmen | teil (nur GET-Liste, `:569-575`) | ja `:470-476` | ja | ja `core/company` | ja `companies.sql` | ja `CompaniesPage.tsx` | ja (Core 77 %, Handler 1 Test) | ja `companies.spec.ts` | voll |
| Deals / Kanban / Widerruf | ja (`:282-320`, ohne `revoke`, `solar-calculation`) | ja `:481-489` | ja `handlers/deals.go` | ja `core/deal` | ja `deals.sql` | ja `DealsPage.tsx` (DnD, Widerruf §355) | ja (Core 58 %, Handler 7) | ja `deals-kanban.spec.ts` | voll; `POST /deals/{id}/solar-calculation` ohne UI (F-1-14) |
| Notizen / Activity-Log / KI-Synthese | ja (`:323-355`) | ja `:505-511`, `:548` | ja `handlers/notes.go` | ja `core/note` | ja `notes.sql` | ja `ActivityLogDrawer.tsx` | teil (Core 54,9 %, 1 Test-Funktion) | ja `notes-activity-log.spec.ts` | teilweise: stiller In-Memory-Fallback (F-1-01), SSE-Leck (F-1-02) |
| Todos | teil (nur GET, `:577`) | ja `:494-500` | ja | ja `core/todo` | ja `todos.sql` | ja `TodosPage.tsx` | teil (Core 51 %, je 1 Lifecycle-Test, `Update` ohne direkten Test) | ja `todos.spec.ts` | voll |
| Termine / Kalender / ICS | ja (`:357-380`, ohne PUT/DELETE/Liste) | ja `:569-575` | ja `handlers/appointments.go` | ja `core/appointment` | ja `appointments.sql` | ja `CalendarPage.tsx` | teil (Core 59,5 %, 2 Test-Funktionen) | ja `calendar-and-telephony.spec.ts` | teilweise: In-Memory-Fallback (F-1-01), SSE-Leck inkl. `is_private` (F-1-02) |
| Telefonie (Click-to-Call, Anruflog) | ja (`:382-396`) | ja `:580-583` (nur `POST /calls`) | ja `handlers/telephony.go` | ja `core/telephony` | ja `call_activities.sql` | ja `CallModal.tsx` (`tel:`-Link + Log) | ja (Core 84 %) | ja (gleiche Spec) | teilweise: reines Protokoll, kein Nutzerbezug, keine Kontaktprüfung, keine Einwilligung (F-1-03, F-1-11) |
| E-Mail / Inbox / Konten / Senden | nein (kein `/emails/*` in der Doku) | ja `:516-529` | ja `handlers/emails.go` | ja `core/email`, `internal/mailclient` (IMAP/SMTP echt) | ja `emails.sql` | ja `InboxPage.tsx` | ja (Core 57 %, mailclient 80 %) | ja `inbox.spec.ts` | voll; Anhänge fehlen/verwaist (F-1-12), Doku fehlt (F-1-06) |
| E-Mail-Templates (inkl. KI-Generierung) | nein | ja `:448-455` (Admin) | ja `handlers/email_templates.go` | ja `core/emailtemplate` (92 %) | ja `email_templates.sql` | ja (Templates in `InboxPage`/`SettingsPage`) | ja | nein | voll, E2E fehlt (F-2-08) |
| Automationen / Workflows / HITL | teil (`:398-412`: List, Runs) | ja `:594-600` | ja `handlers/automations.go` | ja `core/automation` (78 %), Drafter | ja `workflows.sql` | ja `AutomationsPage.tsx` | ja | ja `automations.spec.ts` | teilweise: nur Create/List/Test-Run/Approve, kein Update/Delete/De-/Aktivieren (F-1-10) |
| Reports | nein (nur Überschrift) | ja `:587-590` (nur `/sales`) | ja `handlers/reports.go` | ja `core/reports` (92 %) | kein eigenes SQL (nutzt `ListDeals`/`ListContacts`) | ja `ReportsPage.tsx`, `DashboardPage.tsx` | ja | ja `reports.spec.ts` | teilweise: Fehler verschluckt, stilles 10000er-Limit (F-1-05) |
| Export / Import (CSV) | nein | ja `:614-618` | ja `handlers/export.go` | ja `core/export` (92 %) | nutzt Kontakt-Queries | ja `CSVImportModal.tsx`, Export-Button | ja | ja `notifications-and-export.spec.ts` | voll für Kontakte-CSV; kein Betroffenen-Export (F-1-04) |
| Benachrichtigungen | teil (`:583`) | ja `:605-610` | ja `handlers/notifications.go` | ja `core/notification` (90 %) | ja `notifications.sql` | teil (Header: unread, read-all; kein Einzel-Read) | ja | ja | voll; `POST /{id}/read` ohne UI (F-1-14) |
| KI: Chat / Triage / Research / KB / Parse-Bill / Observability | teil (`:414-445`: Triage, Observability; Chat, Research, KB, parse-bill fehlen) | ja `:533-549` | ja `handlers/ai.go` | ja `internal/ai` (75 %, Guard im Gateway `gateway.go:86,147`) | ja `knowledge_base.sql`, `ai_research_jobs.sql` | ja `AIAssistantPage.tsx`, `AIChatDrawer.tsx` | ja (`ai_test.go`, `kb_embedding_test.go`) | ja `ai-assistant.spec.ts` (Demo-Backend) | voll; Live-Ollama-Test wird übersprungen |
| Connectors / Lead-Intake | ja (`:198-228`) | ja `:345` (Token, Rate-Limit, CSRF-exempt) | ja `handlers/connectors.go` | ja `internal/connectors` (72,7 %) | via Kontakt/Deal | teil (nur Webhook-URL-Anzeige `SettingsPage.tsx:967`) | ja (1 Datei, 174 Zeilen) | ja `connectors.spec.ts` | voll; Token nur per ENV, keine UI-Verwaltung (S4) |
| Karte / Geocoding | nein | nein (kein eigener Endpunkt; nutzt `GET /contacts`) | nein | ja `internal/geocoding` (nur PLZ-Zentroide), Aufruf in `core/contact/service.go:257,414` | Spalten `latitude/longitude` | ja `MapViewPage.tsx` | ja (100 %) | ja `map-view.spec.ts` (1 Test, prüft nur Container + Attribution) | teilweise: nur 310 PLZ, keine Adress-Geokodierung; externe Tile-/Icon-Hosts (F-1-09) |
| Angebotsrechner (Solar) | nein (Doku fehlt) | ja `:554` (`DealsWrite`), `:489` | ja `handlers/calculator.go` | Logik im Handler (kein `core/`-Paket) | `deals.custom_fields` | ja `OfferCalculatorModal.tsx` | ja (`calculator_test.go`, 1 Test) | ja `offer-calculator.spec.ts` | voll, aber Struktur-Abweichung (Logik im Handler) und Rollen-Inkonsistenz (F-1-07) |
| Backup / Restore / Drill | ja (`:532-566`, ohne `restore`) | ja `:418-424` (Admin, Heavy-Limiter) | ja `handlers/backup.go` | ja `core/backup` (85 %) | ja `backup.sql` | ja `SettingsPage.tsx` (Export, Drill, Restore) | ja (umfangreich; Postgres-Integrationstests werden ohne `TEST_DATABASE_URL` übersprungen, F-2-04) | ja `backup-stream.spec.ts` (nur Demo) | voll |
| Einladungen / Nutzer / 2FA / Sessions | ja (`:471-530`, Auth `:43-178`) | ja `:339-340`, `:395-412` | ja `users.go`, `invites.go`, `auth.go`, `session.go` | ja `core/invite` (86 %), `internal/auth` (78 %) | ja `users.sql`, `invites.sql`, `sessions.sql` | ja `SettingsPage.tsx`, `AcceptInvitePage.tsx`, `LoginPage.tsx` | ja | teil (`auth.spec.ts`, TOTP über `page.route`-Mocks, Invite nur teils) | teilweise: kein Passwort-Reset, keine Nutzer-Löschung, `logout-all` ohne UI, Refresh-Token-Cleanup ungenutzt (F-1-13, F-1-15) |
| Einstellungen (Export/Import) | nein | ja `:429-433` (Admin) | ja `handlers/settings.go` | `internal/launcher` (`.env`-Datei) | kein SQL | ja `SettingsPage.tsx` | ja (`settings_test.go`) | nein | teilweise: wirkt nur mit `.env` im Arbeitsverzeichnis, im Docker-Image ohne Wirkung (F-1-08) |
| Eigentümer-Admin (Bulk-Assign) | ja, aber falscher Pfad (`/admin/ownership`, `API_REFERENCE.md:180,185`; Code `/ownership`, `router.go:438`) | ja `:438-441` | ja `handlers/ownership_admin.go` | direkt über Querier + Audit | Queries in `users.sql`/Bulk-Updates | nein (`grep ownership web/src` leer; nur `OwnerSelect.tsx` für Einzel-Zuweisung) | ja (Handler-Tests, Postgres-Test wird übersprungen) | nein | teilweise: API-only (F-1-06, F-1-14) |
| SSE (Realtime) | ja (`:447-470`) | ja `router.go:145-177` (Cookie/Bearer + Token-Version) | Closure im Router | `internal/sse` (86,5 %) | kein SQL | ja `useSSE.ts` | ja (`SendToUser` ohne Test) | nein | teilweise: Nutzer-Filter existiert, wird aber von 4 von 5 Publishern umgangen (F-1-02) |

## 2. Doku-Abgleich (Step 2)

Geprüft: 33 `curl`-Beispiele in `docs/API_REFERENCE.md` gegen `router.go`.

- Doku ohne passende Route: `POST /api/v1/admin/ownership/bulk-assign` (Doku `:185`). Code: `/api/v1/ownership/bulk-assign` (`router.go:438-440`). -> F-1-06.
- Alle übrigen 32 dokumentierten Endpunkte existieren.
- Routen ohne Doku (Auswahl, mehr als die Hälfte der rund 100 Routen): gesamte `/emails/*`, `/email-templates/*`, `/ai/chat`, `/ai/research/*`, `/ai/kb*`, `/ai/parse-bill`, `/calculator/solar`, `/settings/*`, `/export/contacts.csv`, `/import/contacts`, `/reports/sales`, `/backup/restore`, `/deals/{id}/revoke`, `/deals/{id}/solar-calculation`, `/auth/accept-invite`, `/automations` POST, `/automations/{id}/test-run`, `/automations/runs/{id}/approve`, `/notifications/{id}/read`, `/notifications/read-all`, Detail-/PUT-/DELETE-Routen von Companies/Todos/Notes/Appointments.
- Middleware-Kette (Soll/Ist, relevant für Task 3): alle geschützten Routen laufen über `AuthMiddleware` + `CSRFProtectionMiddleware` (`router.go:350-351`); Login nur Rate-Limit; Lead-Intake nur Token + Rate-Limit; Backup/Export/Import zusätzlich `heavyLimiter`.

## 3. Stub-/Fake-Suche (Step 4)

`grep -rniE 'TODO|FIXME|XXX|not implemented|mock|fake|placeholder|hardcod|stub'` über `internal cmd web/src` (ohne Tests, ohne `placeholder=`-Attribute): **keine** `TODO`/`FIXME`/`not implemented`-Treffer. Alle Treffer harmlos: Kommentare "no fakes"/"never creates fake demo accounts" (`mailclient/smtp.go:2`, `automation/draft.go:43`), Substring-Treffer `todo` (Domäne Todos), `launcher/templates.go:173` (Kommentar zu hartkodierten Embedding-Defaults).

Echte Stub-ähnliche Muster wurden stattdessen über Code-Lektüre gefunden: In-Memory-Fallbacks in Produktions-Services (F-1-01), `internal/storage` ohne Produktivnutzer (F-1-12), Settings-Import ohne Wirkung im Container (F-1-08).

Demo-Isolation: `web/src/simulation/SimulationWidget.tsx` wird nur bei `demo_mode` aus `/api/v1/health` gerendert (`AppLayout.tsx:11-12,61-65`, lazy); der Code bleibt als separater Chunk im Produktions-Bundle (S4, F-1-16). Backend: `demo.NewInMemoryQuerier` nur bei `DEMO_MODE=true` (`cmd/server/main.go:206-207`); `IngestDemo` ist in Produktion per Laufzeit-Check gesperrt (`handlers/emails.go:81`), die Route ist aber registriert (`router.go:520`).

## 4. DSGVO / UWG-Produktversprechen (Step 5)

| Anforderung | Befund | Beleg |
|---|---|---|
| Einwilligungsfelder Telefon/E-Mail + Zeitstempel | vorhanden, Audit-Aktion `CONSENT_UPDATE` | `migrations/00004:5-8`, `core/contact/service.go:268-270,433-436,549-550` |
| Einwilligung wird beim Anruf/Versand durchgesetzt (UWG §7) | **fehlt**: kein Treffer für `consent` in `core/telephony`, `core/email`, `handlers/telephony.go`, `handlers/emails.go`, `core/automation`, `CallModal.tsx` | F-1-03 |
| Widerruf (§355 BGB) von Deals | vorhanden (Stage `WIDERRUFEN`, `revoke`-Route, Report `RevokedDeals`) | `router.go:488`, `DealsPage.tsx:74,94`, `core/reports/service.go:19` |
| Löschung / Anonymisierung (Art. 17) | nur Hard-Delete; keine Anonymisierung; Notizen haben keinen FK zum Kontakt und bleiben als Waisen, E-Mail-Nachrichten behalten Absenderdaten | `core/contact/service.go:560-571`, `migrations/00008:2-11`, `00003:45` -> F-1-04 |
| Auskunft / Portabilität (Art. 15/20) | nur Kontakte-CSV (alle Kontakte) und Admin-Voll-Backup; kein personenbezogener Export (Kontakt inkl. Notizen, Mails, Anrufe, Deals) | `core/export/service.go:40` -> F-1-04 |
| Aufbewahrung / Retention | kein Löschjob für Audit, Mails, AI-Logs, Refresh-Tokens (nur Sitzungs-Query existiert, ungenutzt) | `queries/sessions.sql:47`, Grep ohne Aufrufer -> F-1-15 |
| Audit-Log | vorhanden mit Hash-Kette (`00021`), Details Task 4; Fehler beim Audit-Schreiben werden nur geloggt (`core/contact/service.go:566-568`) | Task 4 |

## 5. Findings

### F-1-01 (S2) Stiller In-Memory-Fallback bei DB-Fehlern (Notizen, Termine)
- Beleg: `internal/core/note/service.go:190-231` (Create fällt bei `err != nil` in die In-Memory-Liste, Z. 199 `if err == nil`, danach Z. 215-230 Memory-Pfad und `return n, nil`); `note/service.go:236-256` (Update analog); `internal/core/appointment/service.go:235-305` (Create, Z. 255), `:314-335` (Update, Z. 327), `:354-362` (Delete löscht bei ungültiger UUID nur im Speicher und meldet Erfolg).
- Auswirkung: Bei Postgres-Fehler (Timeout, Constraint, Verbindungsabbruch) antwortet die API mit Erfolg, die Daten liegen nur im Prozessspeicher, sind für andere Nutzer/nach Neustart weg und wachsen unbegrenzt (RAM-Budget). Stiller Datenverlust, Verstoß gegen "keine Fake-Features".
- Fix: Memory-Pfad nur bei `querier == nil` (Demo) zulassen; bei DB-Fehler Fehler zurückgeben (HTTP 500/409). Delete mit ungültiger UUID -> 400/404. Regressionstest mit fehlschlagendem Querier.
- Aufwand: S-M.

### F-1-02 (S2) SSE-Events gehen an alle angemeldeten Nutzer (Inhalte fremder Datensätze)
- Beleg: `hub.Broadcast` ohne Zielnutzer in `core/note/service.go:300-306` (Event enthält komplette Notiz inkl. `content`, Z. 210/230/252), `core/appointment/service.go:375-381` (kompletter Termin inkl. `IsPrivate`, `Notes`, `Location`), `core/email/service.go:150-162` (Betreff, Absender-Mail/-Name, `contact_id`), `core/telephony/service.go:79-87` (Kontakt-ID, Dauer), `core/notification/service.go:60-67` (Broadcast, wenn `UserID == nil`). `Hub.SendToUser` existiert (`sse/hub.go:47`), wird aber nur für Notifications mit Nutzer genutzt und hat keinen Test.
- Auswirkung: Ownership-Scoping (SP8a) wird über SSE umgangen: ein BENUTZER sieht live Notizen-Inhalte, private Termine und Mail-Betreffe fremder Datensätze (DSGVO-Vertraulichkeit). Der Stream wird zwar pro Nutzer authentifiziert (`router.go:145-177`), aber nicht gefiltert.
- Fix: Events mit Owner-ID zielgerichtet per `SendToUser` an Owner + Admins/Rollen mit Team-Sicht senden; Payload auf IDs reduzieren (Client lädt nach). Test: zwei Nutzer, Event darf nur beim Owner ankommen.
- Aufwand: M.

### F-1-03 (S2) UWG-Einwilligung wird nicht durchgesetzt
- Beleg: `consent_phone`/`consent_email` werden nur gespeichert/exportiert (`core/contact/service.go:227-290`, `core/export/service.go:64-108`). Kein `consent` in `core/telephony/service.go`, `handlers/telephony.go`, `core/email/service.go` (`SendMessage` Z. 214), `handlers/emails.go`, `core/automation/*`, `components/telephony/CallModal.tsx` (Anruf-Link `tel:` Z. 120 unabhängig vom Flag). Produktversprechen in `AGENTS.md`: "DSGVO / UWG compliant".
- Auswirkung: Kaltakquise-Anrufe/-Mails an Kontakte ohne Einwilligung sind technisch ohne Warnung/Sperre möglich; Automationen (KI-Entwürfe, Versand nach Freigabe) können dieselben Kontakte adressieren.
- Fix: Server-seitige Prüfung in `telephony.LogCall` (Hinweis/Sperre, überschreibbar mit Begründung + Audit) und in `email.SendMessage`/Automation-Versand; UI-Warnhinweis im `CallModal` und Inbox-Composer. Tests je Pfad.
- Aufwand: M.

### F-1-04 (S2) Betroffenenrechte unvollständig (Art. 15/17/20) und keine Retention
- Beleg: `core/contact/service.go:560-571` (Hard-Delete, Audit-Fehler nur geloggt); `migrations/00008_notes_schema.sql:2-11` (polymorphes `entity_id` ohne FK -> Notizen überleben Kontaktlöschung); `00003:45` (`email_messages.contact_id ON DELETE SET NULL`, Absender-PII bleibt); `core/export/service.go:40` (nur Gesamt-Kontakte-CSV); Grep nach `retention|purge|prune` ohne Produktivtreffer; `AI_COPILOT_AND_COMPLIANCE.md` verspricht DSGVO-Konformität (Abgleich Task 8).
- Auswirkung: Löschanfrage lässt Freitext-PII in Notizen/Mails/Anrufnotizen zurück; Auskunftsanfrage nicht ohne manuelles DB-Wissen beantwortbar; keine Aufbewahrungsfristen.
- Fix: Endpunkt/Service "Betroffenen-Löschung" (Kontakt + abhängige Notizen, Anrufe, Mails, Termine, KB/Embeddings, Anonymisierung der Audit-Diffs) und "Betroffenen-Export" (JSON/ZIP); Retention-River-Job (Audit, Mail, AI-Logs, Sessions); Doku.
- Aufwand: L.

### F-1-05 (S3) Sales-Report verschluckt Fehler und kappt still bei 10000 Zeilen
- Beleg: `core/reports/service.go:42-49` (`ListDeals`-Fehler -> 200 mit leerem Report; `ListContacts`-Fehler mit `_` ignoriert; `Limit: 10000`), Aggregation in Go statt SQL; `handlers/reports.go:18-28`.
- Auswirkung: DB-Ausfall erscheint als "0 Deals"; ab >10000 Datensätzen falsche Kennzahlen ohne Hinweis; Speicherlast.
- Fix: Fehler als 500 zurückgeben; Aggregation per SQL (`GROUP BY stage, month`); Zeitraum-Parameter.
- Aufwand: M.

### F-1-06 (S3) Doku-Drift der API-Referenz
- Beleg: falscher Pfad `/api/v1/admin/ownership/bulk-assign` (`API_REFERENCE.md:180,185` vs. `router.go:438`); über die Hälfte der Routen undokumentiert (Liste in Abschnitt 2).
- Auswirkung: Integratoren laufen in 404; Doku-Zusagen nicht belastbar.
- Fix: Doku korrigieren und vervollständigen, besser OpenAPI aus Router generieren + Test "Route in Doku".
- Aufwand: S (Korrektur) / M (Vollständigkeit).

### F-1-07 (S2) Mehrere definierte Berechtigungen werden nie erzwungen
- Beleg: `internal/auth/rbac.go` definiert `PermContactsWrite`, `PermContactsDelete`, `PermNotesWrite`, `PermEmailsSend`, `PermEmailsTeam`, `PermOwnOnly`; Grep außerhalb `rbac.go` (ohne Tests): 0 Verwendungen. Schreibrouten hängen nur an Lese-Rechten: `router.go:460` (Kontakte inkl. POST/PUT/DELETE), `:471`, `:495`, `:506`, `:517` (inkl. `POST /emails/send`, Z. 521), `:570`. BENUTZER hat laut `rbac.go:95-98` nur `ContactsRead` + `DealsRead`, kann aber Kontakte anlegen/ändern/löschen und Mails senden; gleichzeitig scheitert `POST /calculator/solar` (`router.go:554`, `DealsWrite`) für BENUTZER, obwohl `POST /deals` (nur `DealsRead`, `:482-483`) erlaubt ist. Die Rollen-Matrix-Tests (`server/rbac_routes_test.go:117-173`) prüfen nur 10 Routen.
- Auswirkung: Die "approved role matrix" ist für Schreib-/Sende-Aktionen de facto nicht implementiert; ungewollte Privilegien (Mailversand, Löschen) oder, falls beabsichtigt, tote Berechtigungen und inkonsistentes UI (Rechner).
- Fix: Soll-Matrix festlegen, dann Routen mit `RequirePermission` pro Methode absichern; Matrix-Test auf alle Routen ausweiten (Route-Inventar aus Router generieren).
- Aufwand: M. (Detailbewertung in Task 3.)

### F-1-08 (S3) Einstellungs-Import/-Export ohne Wirkung im Docker-Betrieb
- Beleg: `router.go:429-433`, `settingsH = handlers.NewSettingsHandler()` (ohne `baseDir`, Default `.`, `handlers/settings.go:20-24`); `ImportSettings` schreibt `.env` im Prozess-Arbeitsverzeichnis (`settings.go:140`); `docker-compose.yml` bindet keine `.env` in den `server`-Container ein (Volumes `:45-47`), Meldung "Container bei Bedarf neu starten" (`settings.go:159`).
- Auswirkung: Die UI suggeriert Persistenz; im Standard-Deployment wird eine nie gelesene Container-Datei beschrieben und der Export fällt auf Env-Variablen zurück.
- Fix: Feature im Container deaktivieren/kennzeichnen oder Einstellungen in der DB halten und beim Start lesen.
- Aufwand: M.

### F-1-09 (S3) Karte: Geokodierung nur für 310 PLZ; externe Drittanbieter-Requests
- Beleg: `internal/geocoding/data.go` (310 Einträge; Deutschland hat ca. 8000 PLZ), Nutzung nur `core/contact/service.go:257,414`; `MapViewPage.tsx:141-143` lädt Kacheln von `tile.openstreetmap.org`, `:13-16` Marker-Icons von `unpkg.com`.
- Auswirkung: Die meisten Kontakte landen in "ohne Koordinaten"; Kartennutzung übermittelt Nutzer-IP und Kartenausschnitt an Dritte, im Widerspruch zum Datensouveränitäts-Versprechen (auch CSP/Offline-Betrieb, Task 7).
- Fix: vollständigen PLZ-Datensatz (CC0) einbetten, Marker-Icons lokal bündeln, Tile-Server konfigurierbar/Opt-in dokumentieren.
- Aufwand: M.

### F-1-10 (S3) Workflows nicht verwaltbar
- Beleg: `queries/workflows.sql` enthält nur `CreateWorkflow`, `ListWorkflows`, `GetWorkflowByID`, Run-Queries; Router `:594-600` bietet nur Liste/Create/Test-Run/Approve.
- Auswirkung: Fehlerhafte Workflows können weder bearbeitet, deaktiviert noch gelöscht werden (Notbremse fehlt bei Auto-Versand).
- Fix: `PUT /automations/{id}`, `POST /automations/{id}/toggle`, `DELETE` inkl. Audit, UI-Buttons.
- Aufwand: M.

### F-1-11 (S3) Telefonie-Log ohne Nutzerbezug, Kontaktprüfung und Audit
- Beleg: `core/telephony/service.go:40-70` (nur `contact_id`, Dauer, Disposition, Notizen; keine `user_id`, kein Existenz-/Owner-Check), `handlers/telephony.go:18-37` (keine Claims-Nutzung), Route `router.go:581-582`.
- Auswirkung: Anrufe lassen sich keinem Mitarbeiter zuordnen, Audit fehlt; Notizen auf beliebige Kontakt-IDs möglich (Owner-Scoping umgangen, sofern Rolle Telefonie hat).
- Fix: `user_id` speichern, Kontakt laden und `canModify` prüfen, Audit-Eintrag, 404 bei unbekanntem Kontakt.
- Aufwand: S-M.

### F-1-12 (S4) Dateispeicher ohne Produktivnutzung
- Beleg: `internal/storage` wird nur in `core/email/service.go:39-48` als Feld gehalten (keine Aufrufe), `router.go:125` begrenzt einen Pfad `/api/v1/storage`, der nicht existiert; Tabelle `email_attachments` verwaist (bereits `REVIEW_2026-09-21_round4.md:288`).
- Auswirkung: Toter Code/Angriffsfläche, fehlendes Feature "Anhänge".
- Fix: Anhänge implementieren oder Paket/Tabelle/Body-Limit entfernen.
- Aufwand: M.

### F-1-13 (S3) Kein Passwort-Reset und keine Nutzer-Löschung
- Beleg: Grep `reset|forgot` in `internal/server`, `internal/auth`, `web/src`: keine Treffer; `handlers/users.go` bietet nur Liste/Invite/Rolle/Status (`router.go:409-412`).
- Auswirkung: Vergessenes Passwort = Account muss deaktiviert und neu eingeladen werden (neue E-Mail-Adresse nötig, wenn Unique-Constraint); keine DSGVO-Löschung von Nutzerkonten.
- Fix: Admin-initiierter Reset-Link (Invite-Mechanismus wiederverwenden), Nutzer-Anonymisierung.
- Aufwand: M.

### F-1-14 (S4) API-Funktionen ohne UI
- `POST /ownership/bulk-assign`, `POST /deals/{id}/solar-calculation`, `POST /notifications/{id}/read`, `POST /auth/logout-all` (kein Treffer in `web/src`).
- Fix: UI ergänzen oder Routen entfernen/dokumentieren. Aufwand: S je Route.

### F-1-15 (S3) Refresh-Token-Bereinigung nie aufgerufen
- Beleg: `DeleteExpiredAndSpentRefreshTokens` (`queries/sessions.sql:47`) hat außerhalb von `internal/db` keine Aufrufer (Grep).
- Auswirkung: `refresh_tokens` wächst unbegrenzt (Familien-/Replay-Daten, PII-nahe Metadaten), kein Retention-Nachweis.
- Fix: periodischer River-Job `token_cleanup`. Aufwand: S.

### F-1-16 (S4) Demo-Reste in Produktion
- `POST /emails/demo-ingest` ist registriert (`router.go:520`) und liest `DEMO_MODE` pro Request aus der Umgebung (`handlers/emails.go:81`); Simulation-Chunk im Produktions-Bundle (`AppLayout.tsx:11-12`).
- Fix: Route nur bei `DemoMode` mounten; Chunk per Build-Flag ausschließen. Aufwand: S.

### F-1-17 (S4) Worker-Prozess hat keinen SSE-Hub: Live-Events für synchronisierte Mails fehlen
- Beleg: `cmd/worker/main.go:~55` `email.NewService(queries, auditSvc, nil, nil, automationSvc)` (Storage und `sseHub` = nil); `core/email/service.go:150-162` sendet `email_received` nur bei `sseHub != nil`. Der Hub ist prozesslokal im `crm-server` (`router.go:145`).
- Auswirkung: Per IMAP-Sync (Worker, `internal/queue/jobs.go:108-143`) eingegangene Mails erzeugen kein Live-Event; die UI sieht sie erst beim nächsten Abruf (Header pollt Notifications alle 15 s, `Header.tsx:22`). Das Feature "Live-Updates" gilt daher nur für Demo-Ingest/Server-seitige Pfade.
- Fix: Events über Postgres `LISTEN/NOTIFY` oder `river`-Job an den Server bridgen; bis dahin Doku anpassen.
- Aufwand: M.

## 6. Gesamtbild Task 1

Die Kette UI -> Route -> Handler -> Core -> SQL ist für alle 21 Domänen vorhanden; echte Stubs (Platzhalter-Antworten) wurden nicht gefunden. Die Lücken liegen bei (a) stillem Fallback auf In-Memory (F-1-01), (b) Umgehung des Ownership-Modells über SSE und ungenutzte Berechtigungen (F-1-02, F-1-07), (c) fehlender technischer Umsetzung der Compliance-Versprechen (F-1-03, F-1-04), (d) Doku-Drift (F-1-06).
