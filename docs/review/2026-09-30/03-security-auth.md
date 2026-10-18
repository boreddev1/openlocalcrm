# 03 – Sicherheit A: Auth, Sessions, CSRF, RBAC, Krypto

**Stand:** `main` @ `c6bac3e`, Review am 2026-09-30, read-only (kein Produktcode geändert).
**Grundlage:** Plan `docs/superpowers/plans/2026-09-30-full-source-review-plan.md` Task 3, `AGENTS.md`, Specs SP8a/SP8b/SP8c, `docs/SECURITY_REVIEW_2026-09-21_round3.md`.
**Methode:** Jede Aussage ist per `datei:zeile` belegt. Ein Befund (F-3-05) wurde zusätzlich dynamisch mit einem Test verifiziert, der über `go test -overlay` aus dem Scratchpad eingeblendet wurde (der Repo-Baum blieb unverändert).
**Severity:** S1 kritisch (Datenleck, Auth-Bypass, Datenverlust) · S2 hoch · S3 mittel · S4 niedrig/Hygiene. **Aufwand:** S < 0,5 PT · M ≤ 2 PT · L > 2 PT.

---

## 1. Zusammenfassung

| ID | Sev | Kurztitel | Aufwand |
|---|---|---|---|
| F-3-01 | **S1** | E-Mail-Nachrichten ohne Ownership-/Rollenfilter (Lesen + Taggen fremder Postfächer, auch BENUTZER) | M |
| F-3-02 | **S1** | SSE-`Broadcast` verteilt Notizen, Termine und E-Mail-Metadaten an **alle** Sessions (RBAC-Bypass) | M |
| F-3-03 | **S1** | `PUT /deals/{id}`, `/solar-calculation` und `PUT /todos/{id}` setzen `assigned_to`, `contact_id`, `company_id` bzw. `deal_id` auf NULL (Datenverlust + Owner-Verlust); Create setzt keinen Owner | S |
| F-3-04 | **S1** | Der Master-Key (TOTP-Secrets, Mail-Passwörter) liegt nicht in einem Volume, der Worker nutzt einen eigenen Key → 2FA-Lockout und Credential-Verlust bei jedem Container-Neubau | S |
| F-3-05 | S2 | `/auth/refresh` und `/auth/logout` scheitern immer mit 403 (CSRF-Token ist an den User gebunden, die Route ist anonym) → Logout widerruft nichts, Refresh ist tot | S |
| F-3-06 | S2 | Rollen-/Statuswechsel und Passwortänderung erhöhen `token_version` nicht (SP8a §3.4 nicht umgesetzt) | S |
| F-3-07 | S2 | BENUTZER hat Schreibrechte über die Matrix hinaus (Deals anlegen/ändern, DEAL_WON-Automationen); die Write-Permissions werden nirgends geprüft | M |
| F-3-08 | S2 | Credential-Exfiltration: Ein Team-Mail-Account lässt sich auf einen fremden Host umbiegen, der gespeicherte Login wird beim `/test` gesendet | S |
| F-3-09 | S2 | Das bekannte Default-Admin-Passwort aus `.env.example` wird ohne Prüfung übernommen, `must_change_password` wird nicht erzwungen | S |
| F-3-10 | S3 | TOTP: keine Recovery-Codes, Neu-Enrolment ohne Re-Auth überschreibt den bestehenden Faktor, Replay-Race | M |
| F-3-11 | S3 | CSRF: anonyme Tokens gelten auch für eingeloggte User; das Secret ist pro Prozess zufällig (SP8b-Abweichung) | S |
| F-3-12 | S3 | `ClientIP` vertraut dem linkesten `X-Forwarded-For` jedes privaten Peers; es gibt keine Lockout-Sperre pro Account | S |
| F-3-13 | S3 | Invite-Accept rechnet Argon2id vor der Token-Prüfung → unauthentifizierte Erschöpfung der Argon-Slots (Login-DoS) | S |
| F-3-14 | S3 | Owner-Wechsel werden nicht auditiert; der „Claim-on-Edit“ für Termine/Notizen ist nicht persistiert | S |
| F-3-15 | S3 | ICS-Download autorisiert über das Freitextfeld `assigned_to == email` (von SP8a ausdrücklich verboten) | S |
| F-3-16 | S4 | Notifications: `user_id IS NULL` wirkt global, der Ownership-Check berücksichtigt nur die neuesten 1000 | S |
| F-3-17 | S4 | JWT/Key-Hygiene: `exp` nicht erzwungen, ein unlesbares Keyfile wird still überschrieben | S |
| F-3-18 | S4 | Kontakt-/Firmen-Delete ist nur für ADMIN erlaubt (strenger als die Spec), Last-Admin-Schutz nicht atomar | S |
| F-3-19 | S4 | Tooling: `golangci-lint` v1.64.8 ist mit Go 1.27 nicht auswertbar; govulncheck meldet 1 Modul-Vuln (nicht aufgerufen) | S |

---

## 2. Findings

### F-3-01 · S1 · E-Mail-Nachrichten ohne Ownership-/Rollenfilter

- **Evidenz:**
  - `internal/server/handlers/emails.go:32-50` (`ListMessages`) und `:52-70` (`GetThread`) lesen keine Claims. `:531-557` (`TagMessage`) schreibt auf eine beliebige ID und gibt die komplette Nachricht zurück.
  - `internal/core/email/service.go:181-193` ruft ungefilterte Queries auf.
  - `internal/db/queries/emails.sql:81-89` (`ListEmailMessages`, `ListEmailMessagesByThread`): kein `account_id`/Owner-Prädikat.
  - Die Route-Gruppe verlangt nur `PermContactsRead` (`router.go:515-529`), die alle vier Rollen haben (`rbac.go:62-98`).
- **Impact:** Die Spec SP8a §6.2 sagt „BENUTZER: Personal mailbox only“ und „persönliche Postfächer own-only für alle Nicht-Admins“ (§6.4). Tatsächlich liest jeder BENUTZER alle Nachrichten aller Team- und Personal-Postfächer inklusive Body/HTML. VERTRIEB und BACKOFFICE lesen die persönlichen Postfächer anderer. Per `PATCH /emails/{id}/tags` lassen sich fremde Nachrichten verändern und auslesen (IDOR). Die Konten-Endpunkte sind korrekt abgesichert, die Nachrichten-Endpunkte hebeln das aus.
- **Fix:** In `ListMessages`, `GetThread` und `TagMessage` die Claims auswerten. Die Queries per JOIN auf `email_accounts` einschränken: für BENUTZER `owner_user_id = $caller`, für alle Nicht-Admins zusätzlich `account_type <> 'personal' OR owner_user_id = $caller`. Bei `TagMessage` erst die Nachricht laden, dann `canModifyPersonal` bzw. die Team-Regel prüfen. Tabellen-Test Rolle × Postfachtyp ergänzen.
- **Test-Vorschlag (IDOR):** BENUTZER A, persönlicher Account von B mit einer Nachricht → `GET /emails/messages` darf sie nicht enthalten, `PATCH /emails/{id}/tags` muss 403 liefern.
- **Aufwand:** M

### F-3-02 · S1 · SSE-Broadcast verteilt fremde Daten an alle Sessions

- **Evidenz:**
  - `internal/sse/hub.go:33-44`: `Broadcast` geht an alle Clients, ohne Rollen- oder Owner-Filter.
  - Aufrufer mit vollem Objekt: `internal/core/note/service.go:210,230,252,269` (komplette `Note` inkl. `Content`), `internal/core/appointment/service.go:277,305,333,344,349,375-382` (kompletter Termin inkl. `Notes`, `ContactID`) und `internal/core/email/service.go:152-163` (Betreff, Absender, Kontakt jeder eingehenden Mail).
  - Nur Notifications werden zielgerichtet versendet (`internal/core/notification/service.go:62-66`).
- **Impact:** Ein BENUTZER mit offenem `/events/stream` (`router.go:147-178`) erhält live die Inhalte aller Notizen und Termine sowie die Metadaten aller Postfächer. Das umgeht die Leseeinschränkung der Matrix vollständig. Verschärfend wirkt F-3-06: Rolle/Status werden geändert, ohne `RevokeUser` aufzurufen. Der 60-s-Recheck aus SP8a §3.5 fehlt (`hub.go:128-143`, kein Ticker). Ein deaktivierter Nutzer empfängt also bis zum Verbindungsabbruch weiter.
- **Fix:** `Broadcast` für Fachdaten verbieten. Stattdessen `SendToScope(evt, predicate)` einführen, das pro Client Rolle und Owner prüft (dazu Rolle beim `register` mitspeichern), oder nur IDs senden und den Client über die RBAC-geschützte REST-API nachladen lassen. Den 60-s-Recheck von `token_version` im Stream-Loop implementieren.
- **Aufwand:** M

### F-3-03 · S1 · Deal-/Todo-Update löscht Verknüpfungen und Owner; Create setzt keinen Owner

- **Evidenz:**
  - `internal/server/handlers/deals.go:254-261` und `:348-355` rufen `service.Update` ohne `CompanyID`/`ContactID`/`AssignedTo` auf. `internal/core/deal/service.go:276-285` macht daraus NULL-UUIDs, `internal/db/queries/deals.sql:45-60` schreibt `company_id=$3, contact_id=$4, assigned_to=$9` bedingungslos.
  - Gleiches Muster bei Todos: `internal/server/handlers/todos.go:194-201`, `internal/core/todo/service.go:151-160,176-186`, `internal/db/queries/todos.sql:39-51`.
  - Create: `deals.go:87-92` und `todos.go:44-49` übergeben keinen `AssignedTo`. SP8a §6.4 verlangt aber „The server sets the owner to the creator“.
  - UI-Pfad ist aktiv: `web/src/pages/DealsPage.tsx:172-177` (`PUT /api/v1/deals/{id}`).
- **Impact:** Jede Bearbeitung eines Deals im Kanban (Titel, Stage, Wert) oder das Anhängen einer PV-Kalkulation trennt den Deal dauerhaft von Kontakt und Firma (z. B. aus Lead-Intake, `connector.go:109-115`) und entfernt den Owner. Dasselbe gilt für Todos (Kontakt/Deal/Assignee). Folgen: stiller Datenverlust. Ein BENUTZER verliert nach dem eigenen Update den Zugriff (NULL-Owner ist für BENUTZER unsichtbar, SP8a R5). Neue Deals/Todos sind von Anfang an unowned, für BENUTZER unsichtbar und nur für Team-Rollen änderbar.
- **Fix:** Update-Queries auf `COALESCE(sqlc.narg(...), col)` umstellen oder im Handler die bestehenden Werte (`existing.CompanyID` usw.) übernehmen. Owner-Änderung nur über einen expliziten Pfad mit `canReassign`. Bei Create `AssignedTo = actorID` setzen. Regressionstest: Update erhält `contact_id`, `company_id`, `assigned_to`.
- **Aufwand:** S

### F-3-04 · S1 · Master-Key nicht persistent, Worker-Key divergiert

- **Evidenz:**
  - `internal/crypto/crypto.go:22-31`: Default-Pfad `data/secrets.key`, relativ zum CWD.
  - `docker-compose.yml:29-45,59-62` setzt kein `SECRETS_KEY_PATH`. Volumes existieren nur für `/data/storage` und `/app/keys`.
  - `Dockerfile.server` (Runtime: `WORKDIR /app`) → Key landet unter `/app/data/secrets.key` im Container-Layer.
  - `Dockerfile.worker` (ebenfalls `WORKDIR /app`) erzeugt einen eigenen Key.
  - Außerdem wird ein vorhandener Key bei Lesefehler oder falscher Länge überschrieben: `crypto.go:55-85` (jeder `ReadFile`-Fehler oder jede Nicht-32-Byte-Datei führt zu Neugenerierung und `WriteFile`).
  - Verbraucher: TOTP-Login `internal/auth/service.go:259-267`, Mail `internal/core/email/service.go:220,283,352,408,442`, Worker-Sync `internal/queue/jobs.go:130` über `cmd/worker/main.go:53`.
- **Impact:** Nach `docker compose up --build` oder einem Image-Update entsteht ein neuer Key. Alle TOTP-Secrets werden unentschlüsselbar: `totp_decrypt_failed` → Login unmöglich, auch für Admins. Recovery-Codes und Admin-Reset fehlen (F-3-10). Alle gespeicherten Mail-Passwörter gehen verloren. Der Worker kann die vom Server verschlüsselten Passwörter nie entschlüsseln, der IMAP-Sync-Job schlägt also immer fehl. Das Backup enthält den Key nicht.
- **Fix:** `SECRETS_KEY_PATH=/app/keys/secrets.key` in Compose, Launcher-Template und `.env.example` setzen, den Worker auf dasselbe Volume mounten. Im Produktionsmodus fail-closed wie beim JWT-Key (`cmd/server/main.go:60-62`): ein existierender, aber unlesbarer Key ist ein fataler Fehler und wird nie überschrieben. Key-Backup dokumentieren.
- **Aufwand:** S

### F-3-05 · S2 · Refresh und Logout scheitern immer an CSRF

- **Evidenz:**
  - Login setzt ein **user-gebundenes** CSRF-Token (`internal/server/handlers/auth.go:130`).
  - `/auth/refresh` und `/auth/logout` laufen ohne `AuthMiddleware`, nur mit `CSRFProtectionMiddleware` (`router.go:329-330`). Deshalb ist `GetUserClaims` nil und `ValidateCSRFToken(token, "")` prüft nur `"anonymous"` (`internal/auth/csrf.go:82-85,172-179`) → **403 `csrf_token_invalid`**.
  - Dynamisch bestätigt per Overlay-Test: `POST /api/v1/auth/refresh` mit Login-Token → `status 403 body {"error":"csrf_token_invalid",...}`.
  - Das Frontend verschluckt den Fehler: `web/src/context/AuthContext.tsx:116` (`.catch(() => {})`). Die Unit-Tests rufen die Handler direkt ohne Router auf (`internal/server/handlers/auth_test.go:94-97`). E2E mockt Refresh (`e2e/auth.spec.ts:480`).
- **Impact:**
  1. Logout widerruft das 30-Tage-Refresh-Token nicht und löscht die HttpOnly-Cookies nicht, da die Response 403 ist. Auf geteilten Rechnern bleibt die Session aktiv, ein Reload loggt wieder ein.
  2. Refresh schlägt nach 30 Minuten immer fehl → Zwangs-Logout. Die gesamte in SP8a gebaute Rotations- und Replay-Logik ist im Browser-Pfad tot.
  3. Da der Refresh im Browser immer scheitert, wird der Replay-Schutz nie wirksam getestet.
- **Fix:** Für `/auth/refresh` und `/auth/logout` das CSRF-Token gegen die `user_id` des Refresh-Tokens validieren, oder für diese Routen (SameSite=Strict, Path `/api/v1/auth`) eine Origin/Sec-Fetch-Site-Prüfung statt HMAC-Bindung verwenden. Router-Level-Test Login → Refresh → Logout ohne Mocks.
- **Aufwand:** S

### F-3-06 · S2 · `token_version`-Bump bei Rolle/Status/Passwort fehlt (SP8a §3.4)

- **Evidenz:**
  - `internal/server/handlers/users.go:236-252` (`UpdateRole`) und `:310-326` (`UpdateStatus`) rufen nur `UpdateUserRole`/`UpdateUserStatus` auf. `internal/db/queries/users.sql:40-48` erhöht `token_version` nicht, es gibt keinen Trigger in `internal/db/migrations/*`.
  - `internal/auth/service.go:541-594` (`ChangePassword`): kein `IncrementUserTokenVersion`, kein `DeleteUserRefreshTokens`.
  - Einzige Bump-Stellen: `service.go:158` (logout-all), `:413` (Replay), `:497`/`:532` (TOTP).
  - `RevokeUser` nur in `session.go:40` und `service.go:165,420`.
  - Laut Git-Historie nie implementiert (`git log -S IncrementUserTokenVersion` → nur `9b2f767`, betrifft `service.go`), also keine Regression, sondern ein nicht erfüllter Spec-Punkt.
- **Impact:** Ein herabgestufter Admin behält bis zu 30 min volle Rechte (Rolle kommt aus dem JWT, `rbac.go:117`). Ein deaktivierter Nutzer bleibt bis zu 30 min aktiv, sein SSE-Stream unbegrenzt (F-3-02). Nach einer Passwortänderung wegen Kompromittierung bleiben fremde Refresh-Tokens 30 Tage gültig.
- **Fix:** In `UpdateRole`, `UpdateStatus` und `ChangePassword` in einer Transaktion `IncrementUserTokenVersion` (plus `DeleteUserRefreshTokens` bei Status/Passwort) und danach `sse.Hub.RevokeUser` ausführen. Das Hub-Objekt in `UserHandler` injizieren. Test: Token vor der Rollenänderung → 401.
- **Aufwand:** S

### F-3-07 · S2 · BENUTZER-Schreibrechte über die Matrix hinaus; Write-Permissions unbenutzt

- **Evidenz:**
  - `internal/auth/rbac.go:14-33` definiert `PermContactsWrite`, `PermContactsDelete`, `PermNotesWrite`, `PermEmailsSend`, `PermEmailsTeam`, `PermOwnOnly`. Außer `PermDealsWrite` (nur `/calculator/solar`, `router.go:554`) wird keine davon irgendwo geprüft: `grep` über `internal`/`cmd` findet außerhalb von `rbac.go` und `router.go` keine Verwendung.
  - Schreibrouten hängen nur an Read-Permissions: `router.go:460-465` (contacts), `:471-476` (companies), `:482-489` (deals: `POST /`, `PUT /{id}`, `POST /{id}/solar-calculation` nur `PermDealsRead`), `:495-500`, `:506-511`, `:517-528`, `:570-575`.
  - Spec SP8a §6.2: Deals BENUTZER „Read-only (assigned)“. Tatsächlich darf BENUTZER Deals anlegen (`deals.go:68-102`) und eigene Deals ändern (`canModify` erlaubt dem Owner, `authz.go:34-37`), inklusive Stage → `WON`, was Automationen triggert (`deals.go:267-271`).
  - E-Mail senden aus dem eigenen Account ist ohne `PermEmailsSend` möglich (`emails.go:464-525`).
  - Der RBAC-Tabellentest prüft keine BENUTZER-Schreibfälle (`internal/server/rbac_routes_test.go:109-175`).
- **Impact:** Die in der Spec versprochene Rollenmatrix ist für BENUTZER nicht durchgesetzt. Ein BENUTZER kann Pipeline-Daten erzeugen und manipulieren und über `DEAL_WON` Automationen auslösen (Webhooks, E-Mail-Entwürfe). Die Permission-Map vermittelt eine Scheinsicherheit.
- **Fix:** Pro Schreibroute die passende Write-Permission setzen (`PermDealsWrite` für `POST/PUT /deals*`, `PermContactsWrite` für `POST/PUT /contacts|companies`, `PermNotesWrite`, `PermEmailsSend`). Für BENUTZER entscheiden und dokumentieren, ob eigene Kontakte, Notizen und Termine erlaubt sind („own items only“), dann gezielt freigeben. Den Tabellentest um alle Schreibrouten × 4 Rollen erweitern und unbenutzte Permissions entfernen.
- **Aufwand:** M

### F-3-08 · S2 · Credential-Exfiltration über Host-Umleitung von Mail-Accounts

- **Evidenz:**
  - `internal/server/handlers/emails.go:275-283`: Team-Accounts dürfen alle Nicht-BENUTZER ändern, persönliche Accounts ADMIN oder der Owner.
  - `:291-300` übernimmt `ImapHost`/`SmtpHost`/Ports aus dem Body.
  - `internal/core/email/service.go:400-427` behält das gespeicherte verschlüsselte Passwort, wenn keins mitgesendet wird.
  - `POST /accounts/{id}/test` (`service.go:436-465`) entschlüsselt es und sendet `smtp.PlainAuth` bzw. IMAP `LOGIN` an den neuen Host (`internal/mailclient/smtp.go:102-107`, `imap.go:88`). TLS wird verlangt, ein Angreifer-Host mit gültigem Let's-Encrypt-Zertifikat genügt.
- **Impact:** Ein VERTRIEB/BACKOFFICE-Nutzer kann das Passwort jedes Team-Postfachs abgreifen, ein ADMIN das Passwort jedes persönlichen Postfachs. Credential-Diebstahl über Rollengrenzen hinweg.
- **Fix:** Bei Änderung von Host, Port oder Username das Passwort zwingend neu verlangen (sonst gespeichertes Secret verwerfen). Team-Accounts nur für ADMIN änderbar. Änderungen auditieren (siehe F-4-08).
- **Aufwand:** S

### F-3-09 · S2 · Bekanntes Default-Admin-Passwort wird akzeptiert

- **Evidenz:**
  - `.env.example:14`: `INITIAL_ADMIN_PASSWORD=change_this_secure_admin_password_123!`, laut `AGENTS.md` „Copy `.env.example` to `.env`“.
  - `cmd/server/main.go:80-99` übernimmt jeden nicht-leeren Wert ohne Blocklist oder Mindestlänge.
  - `must_change_password` wird nur gemeldet (`router.go:381`), die API ist nicht gesperrt (SP8a §1.1, R4).
  - Analog `CONNECTOR_API_TOKEN=your_secure_connector_token_here` (`.env.example:34`) ohne Entropieprüfung (`internal/connectors/connector.go:68`).
- **Impact:** Eine Installation mit unveränderter Vorlage hat öffentlich bekannte Admin-Credentials. Bei unverändertem Connector-Token kann jeder Leads einspielen.
- **Fix:** Beim Bootstrap Platzhalterwerte aus `.env.example` und Passwörter unter 12 Zeichen ablehnen (fatal im Produktionsmodus). Dasselbe für den Connector-Token (Mindestlänge 32, kein Platzhalter). Optional `must_change_password` API-seitig gaten (Whitelist `/me`, `/auth/change-password`, `/auth/logout`).
- **Aufwand:** S

### F-3-10 · S3 · TOTP: keine Recovery-Codes, Enrolment ohne Re-Auth, Replay-Race

- **Evidenz:**
  - `internal/server/handlers/auth.go:293-314` / `internal/auth/service.go:473-504`: `VerifyTOTP` akzeptiert ein vom Client geliefertes `secret` und überschreibt ein bereits aktives TOTP ohne Passwort und ohne aktuellen Code.
  - Keine Recovery-Codes, kein Admin-Reset-Endpunkt (Router `router.go:393-402`).
  - Login: Code prüfen (`service.go:269`), danach separat `UpdateUserTOTPLastUsedStep` (`:279-284`) → zwei parallele Logins mit demselben Code sind möglich.
- **Impact:** Wer eine Session erbeutet hat (XSS, Session-Hijack), kann den 2FA-Faktor auf sein eigenes Secret umstellen und den Inhaber beim nächsten Login aussperren. Ohne Recovery bleibt nur ein manueller DB-Eingriff, auch bei F-3-04. Replay im selben 30-s-Fenster ist möglich.
- **Fix:** Enrolment und Wechsel nur mit Passwort (und bei aktivem TOTP mit aktuellem Code). Das Secret serverseitig aus `SetupTOTP` binden (verschlüsselt, kurzlebig) statt vom Client übernehmen. Recovery-Codes (gehasht) einführen, dazu einen Admin-Reset mit Audit. `last_used_step` atomar setzen: `UPDATE … WHERE totp_last_used_step < $step RETURNING`.
- **Aufwand:** M

### F-3-11 · S3 · CSRF-Bindung wirkungslos, Secret nicht wie spezifiziert

- **Evidenz:**
  - `internal/auth/csrf.go:82-85`: Ein anonymes Token wird für jeden authentifizierten User akzeptiert, die User-Bindung ist damit optional.
  - `:20-33`: Das Secret ist pro Prozess zufällig. SP8b §2.1 fordert die Ableitung aus dem Ed25519-Key.
  - `SetCSRFSecret` (`:36-38`) schreibt ohne Lock.
- **Impact:** Nach einem Server-Neustart sind alle CSRF-Cookies ungültig. Jede Mutation liefert 403 und kann nur per Neu-Login geheilt werden (Refresh ist kaputt, F-3-05). Die Bindung schützt nicht gegen Cookie-Injection über Subdomains. Das Risiko ist niedrig, weil der Access-Cookie SameSite=Strict ist (`handlers/auth.go:115`).
- **Fix:** Secret per HKDF aus dem Ed25519-Private-Key ableiten. Den Anonymous-Fallback nur für nicht authentifizierte Routen zulassen (Parameter „allowAnonymous“).
- **Aufwand:** S

### F-3-12 · S3 · IP-Vertrauen und fehlender Account-Lockout

- **Evidenz:**
  - `internal/auth/limiter.go:227-249`: `X-Forwarded-For[0]` wird übernommen, sobald `RemoteAddr` loopback, privat oder link-local ist.
  - Rate-Limit-Key beim Login ist `ip:email` (`internal/auth/service.go:206`), Middleware pro IP (`limiter.go:252-268`).
- **Impact:** Hinter Caddy ist das korrekt, da Caddy fremde XFF-Werte nicht übernimmt. Bei LAN-Betrieb ohne Proxy (`make run-local`, direkter Zugriff auf :8080 aus einem privaten Netz, typisch für On-Prem-KMU) kann jeder Client per XFF beliebige IPs vortäuschen: unbegrenztes Passwort- und TOTP-Raten, gefälschte IPs im Audit-Log. Einen Account-Lockout über IPs hinweg gibt es nicht.
- **Fix:** Die Liste der Trusted Proxies konfigurierbar machen (Default: nur explizit gesetzte CIDRs), das rechteste nicht-vertrauenswürdige XFF-Element verwenden. Zusätzlich einen Fehlerzähler pro Account mit exponentiellem Backoff einführen.
- **Aufwand:** S

### F-3-13 · S3 · Invite-Accept: Argon2id vor Token-Prüfung

- **Evidenz:** `internal/core/invite/service.go:297-318` hasht das Passwort mit `HashPasswordLimited` (64 MiB, 4 globale Slots, `internal/auth/service.go:35-71`), bevor das Token in `AcceptUserInvite` geprüft wird. Die Route ist öffentlich (`router.go:340`), nur `loginLimiter` 5/min pro IP greift.
- **Impact:** Mit wenigen IPs bleiben alle 4 Argon2-Slots dauerhaft belegt → Logins warten bzw. brechen per Context ab (Login-DoS).
- **Fix:** Zuerst die Existenz des Tokens günstig per SHA-256-Lookup prüfen (`resolveActiveInvite`), erst danach hashen. Optional einen eigenen Semaphore für den Invite-Pfad.
- **Aufwand:** S

### F-3-14 · S3 · Owner-Änderungen nicht auditiert; Claim-on-Edit nicht persistiert

- **Evidenz:**
  - `internal/core/contact/service.go:321-327` und `internal/core/company/service.go:93-99` (`SetOwner`) haben keinen Audit-Eintrag. Aufruf aus `contacts.go:236-246` und `companies.go:186-195`.
  - Termine: Der Handler setzt `input.UserID` bei NULL-Owner (`appointments.go:136-139`), `UpdateAppointment` schreibt `user_id` aber nicht (`internal/core/appointment/service.go:316-326`).
  - Notizen claimen gar nicht (`notes.go:162`). SP8a §2.1/§6.4 verspricht Auto-Claim.
- **Impact:** Die Übertragung von Kundenbeständen zwischen Vertrieblern ist nicht nachvollziehbar (Compliance). Unowned-Altbestände heilen nicht und bleiben für BENUTZER unsichtbar.
- **Fix:** `SetOwner` mit `LogDiff(owner_user_id alt/neu)`. `user_id` in `UpdateAppointment`/`UpdateNote` per `COALESCE` setzen.
- **Aufwand:** S

### F-3-15 · S3 · ICS-Autorisierung über Freitext

- **Evidenz:** `internal/server/handlers/appointments.go:199-207`: `isOwner` ist wahr, wenn `app.AssignedTo == claims.Email`. SP8a §6.3: „`appointments.assigned_to` … is **never** an authorization input“. Team-Rollen (VERTRIEB/BACKOFFICE) werden abgewiesen, obwohl sie den Termin per `GET /appointments` sehen dürfen.
- **Impact:** Jeder mit Schreibrecht auf einen Termin kann `assigned_to` auf die E-Mail eines Dritten setzen und damit Exportrechte vergeben. Dazu kommt die inkonsistente Read-Policy.
- **Fix:** `canRead(claims, ownerUUID)` verwenden, den Freitext-Vergleich entfernen.
- **Aufwand:** S

### F-3-16 · S4 · Notifications: globale NULL-User-Einträge

- **Evidenz:**
  - `internal/db/queries/notifications.sql:15-36`: `user_id = $1 OR user_id IS NULL` in List und `MarkAll`.
  - `internal/core/notification/service.go:86-110`: Der Ownership-Check durchsucht nur die neuesten 1000 Einträge (`Limit: 1000`), das Update `MarkNotificationAsRead` hat kein `user_id`-Prädikat (`notifications.sql:28-31`).
- **Impact:** Ein Nutzer markiert globale Hinweise für alle als gelesen. TOCTOU/Fenster-Effekte, aber kein Datenleck.
- **Fix:** Gelesen-Status für Broadcasts pro User in einer Join-Tabelle führen. `UPDATE … WHERE id=$1 AND user_id=$2`.
- **Aufwand:** S

### F-3-17 · S4 · JWT-/Key-Hygiene

- **Evidenz:**
  - `internal/auth/jwt.go:48-53`: kein `jwt.WithExpirationRequired()`, kein `nbf`.
  - `cmd/server/main.go:33-57`: Ist das Keyfile vorhanden, aber unlesbar oder falsch groß, wird es überschrieben (alle Sessions ungültig).
- **Impact:** Gering, da nur der Server signiert. Bei einem Rechteproblem am Volume gehen jedoch still alle Sessions verloren.
- **Fix:** `WithExpirationRequired()`, `WithLeeway(30s)`. Bei vorhandenem, aber unbrauchbarem Keyfile fail-closed statt überschreiben.
- **Aufwand:** S

### F-3-18 · S4 · Delete-Semantik und Last-Admin-Race

- **Evidenz:**
  - `contacts.go:279-284` und `companies.go:222-227`: Löschen nur für ADMIN. Spec §6.2: VERTRIEB/BACKOFFICE „Full CRUD“.
  - `users.go:221-234,295-308`: Die Prüfung auf den letzten Admin zählt außerhalb einer Transaktion.
- **Impact:** Abweichung zur Spec, zu restriktiv. Zwei parallele Admin-Demotions können den letzten Admin entfernen.
- **Fix:** Spec oder Code angleichen. Last-Admin-Prüfung atomar (`UPDATE … WHERE (SELECT count(*) …) > 1`).
- **Aufwand:** S

### F-3-19 · S4 · Scanner-Tooling

- **Evidenz:**
  - `golangci-lint has version v1.64.8` meldet mit Go 1.27.1 14× `typecheck`-Fehler („export data version 4 is greater than maximum supported version 2“, `undefined: chi/pgx/jwt`). Die Ergebnisse sind nicht auswertbar.
  - `govulncheck ./...`: 0 aufgerufene Vulns. Modul-Ebene: `GO-2026-5932` (`golang.org/x/crypto@v0.57.0`, Paket `openpgp` unmaintained, wird nicht importiert, kein Fix verfügbar).
  - `go vet` auf `internal/auth`, `internal/server/...`, `internal/ai`, `internal/core/...`: sauber.
- **Impact:** Das Lint-Gate liefert lokal keine Aussage (CI-Version in Task 8 prüfen).
- **Fix:** golangci-lint v2 (mit Go ≥ 1.26 gebaut) pinnen.
- **Aufwand:** S

**gosec-Triage** (148 Issues, `Nosec: 0`, 3× `//nolint:gosec` in `internal/mailclient/{smtp.go:78,92, imap.go:67}` – begründet, `InsecureSkipVerify` wird nur von Tests gesetzt; Produktionspfade in `internal/core/email/service.go:224-230,289-295,447-463` setzen es nie):

- **G115/G109** (30×, `strconv.Atoi` → `int32` in List-Handlern): Überlauf ergibt negative oder große Limits. Die Services klemmen die Werte (`deal/service.go:226`, `todo/service.go:116`, `email/service.go:182`), Contacts/Companies wurden nicht geprüft → Hygiene (Task 6).
- **G101** (19×): False Positives (sqlc-Queries mit „token“/„password“ im Namen, Default-DSN in `cmd/server/main.go:143`, `cmd/worker/main.go:37` → siehe F-3-09/Task 8).
- **G703/G304/G301/G204/G702**: überwiegend Launcher (lokales Operator-Tool, Task 8). `internal/crypto/crypto.go:55` siehe F-3-04.
- **G124** (8×): Die Cookies setzen Secure dynamisch (`isSecureRequest`). Das ist korrekt, weil hinter Caddy `X-Forwarded-Proto` gesetzt wird.
- **G705** `appointments.go:214`: False Positive (`text/calendar`, escapte Felder `appointment/service.go:384-392`).
- **G120** `export.go:42`: Durch die globale 5-MB-`MaxBytesReader` begrenzt (`router.go:119-131`).
- **G110** `launcher/updater.go:276`: Task 8.

---

## 3. RBAC-/Ownership-Matrix (Route → Soll → tatsächlich erzwungen)

**Permission → Rollen** (`internal/auth/rbac.go:36-99`):

| Permission | Rollen |
|---|---|
| users.admin, backup.admin, settings.export, email_templates.admin, ai.observability, automations.write | ADMIN |
| contacts.read, deals.read | ADMIN, VERTRIEB, BACKOFFICE, BENUTZER |
| deals.write, deals.revoke, contacts.import, telephony.use, ai.use, reports.view, automations.read | ADMIN, VERTRIEB, BACKOFFICE |
| deals.delete, automations.run | ADMIN, VERTRIEB |
| reports.export | ADMIN, BACKOFFICE |

Legende Ownership-Helfer (`internal/server/handlers/authz.go`):

- **canRead/canModify**: A/V/B dürfen alles, BENUTZER nur Datensätze mit `owner == self`.
- **canReassign**: ADMIN oder der aktuelle Owner.
- **canModifyPersonal**: ADMIN oder der Owner.

Alle Routen unter `protected` laufen durch `AuthMiddleware` (JWT + `token_version`, `router.go:350`) und `CSRFProtectionMiddleware` (`:351`).

| Methode & Pfad | Router-Gate (`router.go`) | Soll (SP8a §6.2) | Handler-Prüfung / Ownership | Status |
|---|---|---|---|---|
| GET /health | public :135 | public | – | OK |
| GET /events/stream | JWT + Version bei Connect :147-178 | alle, eigene Events | keine Event-Filterung | **✗ F-3-02** |
| POST /auth/login | loginLimiter :328 | public | – | OK |
| POST /auth/refresh | CSRF + authLimiter :329 | Cookie | CSRF-Bindung scheitert | **✗ F-3-05** |
| POST /auth/logout | CSRF :330 | Cookie | CSRF-Bindung scheitert | **✗ F-3-05** |
| GET/POST /auth/accept-invite | loginLimiter :339-340 | public | Token-Hash, atomar | OK (F-3-13) |
| POST /connectors/lead-intake | connectorLimiter :345 | Bearer | `subtle.ConstantTimeCompare` `connector.go:68` | OK |
| GET /me | JWT :357 | alle | nur eigene Claims | OK |
| POST /auth/change-password | authLimiter :395 | self | `claims.UserID` | OK (kein Version-Bump, F-3-06) |
| POST /auth/totp/setup, /verify, /disable | authLimiter :396-398 | self | `claims.UserID` | OK (F-3-10) |
| POST /auth/logout-all | authLimiter :400 | self | Revoke + Bump `session.go:24-46` | OK |
| GET /users/, POST /users/invite | users.admin :408-410 | ADMIN | + Handler `Role=="ADMIN"` `users.go:107` | OK |
| PUT /users/{id}/role, /status | users.admin :411-412 | ADMIN | Handler ADMIN `users.go:190,264` | OK Zugriff; **✗ kein Revoke F-3-06** |
| POST /backup/drill, GET /backup/export, POST /backup/restore | backup.admin + heavy :419-423 | ADMIN | `adminClaims` `backup.go:48-51` | OK (F-4-01) |
| GET /settings/export, POST /settings/import | settings.export :430-432 | ADMIN | Handler ADMIN `settings.go:38,114` | OK |
| POST /ownership/bulk-assign | users.admin :439-440 | ADMIN | Handler ADMIN `ownership_admin.go:42` | OK |
| GET/POST /email-templates/, POST /generate, PUT/DELETE /{key} | email_templates.admin :449-454 | ADMIN | – | OK |
| POST /contacts/ | contacts.read :460-461 | A/V/B CRUD, BEN read(+own) | Owner = Ersteller `contact/service.go:302-311` | **△ BEN darf anlegen (F-3-07)** |
| GET /contacts/ | contacts.read :462 | BEN nur eigene | Owner-Filter `contacts.go:117-120` | OK |
| GET /contacts/{id} | contacts.read :463 | BEN nur eigene | `canRead` `contacts.go:176` | OK |
| PUT /contacts/{id} | contacts.read :464 | BEN own | `canModify` :207 + `canReassign` :237 | OK (Reassign nicht auditiert F-3-14) |
| DELETE /contacts/{id} | contacts.read :465 | A/V/B | nur ADMIN `contacts.go:281` | OK (strenger, F-3-18) |
| POST /companies/ | contacts.read :471-472 | wie Contacts | Owner = Ersteller `company/service.go:76` | **△ F-3-07** |
| GET /companies/, GET /companies/{id} | contacts.read :473-474 | BEN nur eigene | Filter :81-84, `canRead` :126 | OK |
| PUT /companies/{id} | contacts.read :475 | BEN own | `canModify` :157, `canReassign` :187 | OK |
| DELETE /companies/{id} | contacts.read :476 | A/V/B | nur ADMIN :224 | OK (F-3-18) |
| POST /deals/ | deals.read :482-483 | BEN read-only | kein Owner gesetzt `deals.go:87-92` | **✗ F-3-07, F-3-03** |
| GET /deals/ | deals.read :484 | BEN assigned | Filter `deals.go:110-113` | OK |
| GET /deals/{id} | deals.read :485 | BEN assigned | `canRead` :167 | OK |
| PUT /deals/{id} | deals.read :486 | BEN read-only, BO edit | `canModify` :211 (BEN-Owner darf) | **✗ F-3-07; Datenverlust F-3-03** |
| DELETE /deals/{id} | deals.delete :487 | A/V (BO kein Delete) | `canModify` :297 | OK |
| POST /deals/{id}/revoke | deals.revoke :488 | A/V/B | `canModify` :389 | OK |
| POST /deals/{id}/solar-calculation | deals.read :489 | BEN read-only | `canModify` :338 | **✗ F-3-07, F-3-03** |
| POST /todos/ | contacts.read :495-496 | BEN own | kein Owner gesetzt `todos.go:44-49` | **✗ F-3-03** |
| GET /todos/, GET /todos/{id} | contacts.read :497-498 | BEN own | Filter :71-74, `canRead` :116 | OK |
| PUT /todos/{id} | contacts.read :499 | BEN own | `canModify` :160 | OK Zugriff; **✗ Datenverlust F-3-03** |
| DELETE /todos/{id} | contacts.read :500 | BEN own | `canModify` :230 | OK |
| POST /notes/ | contacts.read :506-507 | BEN own | Owner = Ersteller `notes.go:101-106`; `entity_id` ungeprüft | △ (BEN kann an fremde Entität anhängen) |
| GET /notes/ | contacts.read :508 | BEN own | Filter `notes.go:43-46` | OK |
| GET /notes/{id} | contacts.read :509 | BEN own | `canRead` `notes.go:83` | OK |
| PUT, DELETE /notes/{id} | contacts.read :510-511 | BEN own | `canModify` :151, :189 | OK |
| GET /emails/messages | contacts.read :518 | BEN nur eigenes Postfach | **keine** | **✗ F-3-01** |
| GET /emails/threads/{threadID} | contacts.read :519 | wie oben | **keine** | **✗ F-3-01** |
| POST /emails/demo-ingest | contacts.read :520 | nur Demo | Demo-Flag `emails.go:81` | OK (F-4-17) |
| POST /emails/send | contacts.read :521 | Team + eigenes Postfach | Account-Check `emails.go:496-504`; `From` frei wählbar | △ (From-Spoofing) |
| PATCH /emails/{id}/tags | contacts.read :522 | wie oben | **keine** (liefert Nachricht zurück) | **✗ F-3-01** |
| GET /emails/accounts | contacts.read :523 | BEN nur eigene | Filter `emails.go:219-236` | OK |
| POST /emails/accounts | contacts.read :524 | – | Owner = Ersteller, BEN → personal | OK |
| PATCH /emails/accounts/{id} | contacts.read :525 | personal: own; Team: A/V/B | `canModifyPersonal` / Nicht-BEN :275-283 | **✗ Credential-Exfil F-3-08** |
| DELETE /emails/accounts/{id} | contacts.read :526 | personal own; Team ADMIN | :337-345 | OK |
| POST /emails/accounts/{id}/test, /sync | contacts.read :527-528 | wie PATCH | :379-387, :427-435 | OK Zugriff; SSRF F-4-06 |
| POST /ai/triage, /chat, /research/company, /parse-bill | ai.use + aiLimiter :534-540 | A/V/B | – | OK (PII F-4-03) |
| GET /ai/observability | ai.observability :539 | ADMIN | – | OK |
| GET /ai/kb/search, GET/POST /ai/kb | ai.use :541-543 | A/V/B | Team-Ressource | OK |
| DELETE /ai/kb/{id} | ai.use :544 | A/V/B | kein Owner | △ Team darf löschen, kein Audit |
| GET/POST /ai/research/jobs | ai.use :545-546 | A/V/B | – | OK |
| POST /ai/synthesize-notes | ai.use :548 | A/V/B | Team liest alle Notizen | OK |
| POST /calculator/solar | deals.write :554 | A/V/B | zustandslos | OK |
| GET /appointments/ | contacts.read :571 | BEN own | Filter `appointments.go:34-37` | OK |
| POST /appointments/ | contacts.read :572 | BEN own | Owner = Ersteller `appointment/service.go:253` | OK (F-4-04) |
| PUT, DELETE /appointments/{id} | contacts.read :573-574 | BEN own | `canModify` :125, :168 | OK (Claim fehlt F-3-14) |
| GET /appointments/{id}/ics | contacts.read :575 | wie Read | ADMIN oder Owner oder `assigned_to==email` | **✗ F-3-15** |
| POST /telephony/calls | telephony.use :581-582 | A/V/B | `contact_id` ungeprüft (Team), kein Akteur | OK (Audit F-4-08) |
| GET /reports/sales | reports.view :588-589 | A/V/B | Aggregat | OK |
| GET /automations/, GET /automations/runs | automations.read :596-597 | A/V/B | – | OK |
| POST /automations/ | automations.write :598 | ADMIN | – | OK |
| POST /automations/{id}/test-run | automations.run :599 | A/V | – | OK |
| POST /automations/runs/{id}/approve | automations.run :600 | A/V | kein Akteur, nicht atomar | △ F-4-16 |
| GET /notifications/unread | contacts.read :607 | own | eigene + NULL | OK (F-3-16) |
| POST /notifications/{id}/read | contacts.read :608 | own | Check im 1000er-Fenster | △ F-3-16 |
| POST /notifications/read-all | contacts.read :609 | own | + globale NULL | △ F-3-16 |
| GET /export/contacts.csv | reports.export + heavy :616 | A, B | kein Audit, 10k-Cap | △ F-4-08/F-4-11 |
| POST /import/contacts | contacts.import + heavy :617 | A/V/B | Akteur immer NULL | △ F-4-08 |

**IDOR-Testfall-Vorschläge je Lücke:**

1. F-3-01: BENUTZER liest bzw. taggt die Nachricht eines fremden persönlichen Accounts → 403 erwartet.
2. F-3-02: BENUTZER-Stream darf `note.created` einer fremden Notiz nicht empfangen.
3. F-3-03: `PUT /deals/{id}` mit `{title}` erhält `contact_id`/`assigned_to`.
4. F-3-07: BENUTZER `POST /deals` → 403, `PUT /deals/{own}` → 403.
5. F-3-15: VERTRIEB `GET /appointments/{id}/ics` → 200, BENUTZER mit `assigned_to=eigene Mail` auf fremdem Termin → 403.
6. F-3-08: Nach `PATCH` des `smtp_host` ohne Passwort darf `/test` das alte Secret nicht senden.

---

## 4. Verifiziert OK (mit Evidenz)

- **Argon2id-Parameter** m=64 MiB, t=3, p=2, Salt 16 B, Key 32 B (`internal/auth/password.go:14-20`). Das liegt über dem OWASP-Minimum (19 MiB/t=2/p=1). Vergleich in konstanter Zeit (`:76`). Argon-Semaphore mit 4 Slots für das RAM-Budget (`internal/auth/service.go:35-71`).
- **User-Enumeration:** Dummy-Hash bei unbekannter E-Mail (`service.go:40-46,214-226`), einheitliche Fehlermeldung, Status-Prüfung erst nach dem Passwort (`:235-249`). Der Invite-Validate-Fehler ist generisch (`invites.go:119-131`).
- **JWT:** Nur EdDSA zugelassen (`jwt.go:48-51`), `iss`/`aud` geprüft (`:53`), `token_version` Pflicht (`:60`). Der Version-Check pro Request ist fail-closed (`middleware.go:92-103`), ebenso bei SSE-Connect (`router.go:168-174`). Cookie hat Vorrang vor Bearer nur ohne Header; das CSRF-Bypass gilt nur für Bearer ohne Cookie (`middleware.go:106-109`).
- **Refresh-Rotation** (Logik): transaktional, Replay → Family-Revoke + Version-Bump + `RevokeUser` + Audit (`service.go:336-434`). Wegen F-3-05 im Browser nicht erreichbar.
- **Cookies:** `access_token` HttpOnly, SameSite=Strict, Path `/`. `refresh_token` Path `/api/v1/auth`, Strict. Secure über TLS, `X-Forwarded-Proto` oder `FORCE_SECURE_COOKIES` (`handlers/auth.go:16-27,108-128`).
- **Ed25519-Key:** Dateirechte 0600, Verzeichnis 0700 (`cmd/server/main.go:47-51`). Im Produktionsmodus fail-closed, wenn nicht persistierbar (`:60-62`). Volume `crm_keys` (`docker-compose.yml:44`).
- **CSRF:** Double-Submit mit `subtle.ConstantTimeCompare` plus HMAC (`csrf.go:167-179`). `internal/server/csrf.go` ist ein reiner Wrapper ohne Logik-Duplikat (`:14-24`). Die Ausnahmen sind begründet: Login (`csrf.go:143`), Accept-Invite (keine Session, rate-limitiert, `router.go:333-341`), Lead-Intake (Bearer, `router.go:343-346`).
- **TOTP:** Secret AES-256-GCM mit Zufalls-Nonce (`internal/crypto/crypto.go:100-125`). Fenster ±1 Step mit `last_used_step` (`totp.go:25-51`). `SetupTOTP` speichert nichts (SP8a F-03, `service.go:449-471`). Enable/Disable erhöhen die Version (`:497,:532`). Disable verlangt das Passwort (`:506-521`).
- **Admin-Routen** sind doppelt abgesichert (Router-Permission plus Handler-Rollencheck): `users.go:107,190,264`, `backup.go:48-51`, `settings.go:38,114`, `ownership_admin.go:42`.
- **Invites:** Token 32 B Zufall, SHA-256-gehasht (`invite/service.go:134-148`), TTL 72 h (`:37`), atomares Accept (`:313-318`). Die Response-Projektion enthält keinen Hash (`invites.go:29-37`). Der Link-Host wird sanitisiert (`invites.go:157-166`).
- **Bootstrap:** Das generierte Passwort wird nicht geloggt, sondern als 0600-Datei geschrieben (`cmd/server/main.go:71-94`) und bei der ersten Passwortänderung gelöscht (`service.go:583-585`).
- **Secrets im Repo:** `git log --all -S'PRIVATE KEY'` findet nur Test-generierte Keys (`mailclient_test.go`) und die Hook-Regex. `.env`, `/data/` und `/backups/` sind ignoriert (`git check-ignore`). Keine getrackten `*.key`/`*.pem`/`.env`. Kein `math/rand`, `md5`, `sha1` oder hart gesetztes `InsecureSkipVerify: true` in `internal`/`cmd`.
- **govulncheck:** keine aufgerufenen Schwachstellen.

## 5. Abgleich früher gemeldeter Fixes (Round 3, SP8a, SP8b, SP8c – Auth-Teil)

| Früherer Punkt | Ergebnis | Evidenz |
|---|---|---|
| R3 N1/N2 Deal-IDOR | **bestätigt behoben** | `canModify` `deals.go:211,338`; zusätzlich Delete/Revoke `:297,389` |
| R3 N17 keine Rollen auf Kern-Routen | **teilweise**: Read-Permissions gesetzt, Write-Permissions nie geprüft | F-3-07 |
| R3 N8 Notification-MarkRead-IDOR | **behoben** (mit Fenster-Einschränkung) | `notification/service.go:86-106`; F-3-16 |
| R3 M2 In-Memory-Revocation | **behoben** über `token_version` | `middleware.go:92-103` |
| R3 M3 XFF-Spoofing | **behoben für Docker/Caddy**, offen für LAN ohne Proxy | F-3-12 |
| R3 N16 Connector-Token in localStorage | **behoben** | `grep localStorage web/src` ohne Treffer |
| SP8a §3.1 Version-Check pro Request | **bestätigt** | `middleware.go:92-103`, `router.go:168-174` |
| SP8a §3.2 Refresh-Familien/Replay | **Code vorhanden, im Browser unerreichbar** | F-3-05 |
| SP8a §3.4 Bump bei Rolle/Status/Passwort | **nicht umgesetzt** (nie implementiert) | F-3-06 |
| SP8a §3.4 Bump bei TOTP an/aus, logout-all | **bestätigt** | `service.go:158,497,532`, `session.go:33-41` |
| SP8a §3.5 SSE 60-s-Recheck | **nicht umgesetzt** | `hub.go:128-143` |
| SP8a §5 Bootstrap-Datei statt Log | **bestätigt** | `main.go:71-94` |
| SP8a §6.3 fail-closed Claims | **bestätigt** | `authz.go:11-19`, alle Handler |
| SP8a §6.4 Creator owns | **Contacts/Companies/Notes/Appointments ja, Deals/Todos nein** | F-3-03 |
| SP8a §6.4 Claim-on-Edit | **nicht wirksam** | F-3-14 |
| SP8a F-03 TOTP-Setup-Race | **bestätigt** | `service.go:449-504` |
| SP8b §2.1 HMAC-CSRF | **teilweise**: HMAC ja, Secret-Ableitung nein, Anonymous-Fallback | F-3-11 |
| SP8b §2.1 Cookie-Attribute | **bestätigt** | `csrf.go:114-130`, `handlers/auth.go:108-128` |
| SP8c Security-Event-Logging (Login, Refresh-Reuse, Logout, TOTP, Rolle/Status) | **bestätigt** (Schreibpfade) | `service.go:199-327,425-433`, `users.go:245-252,319-326` |
