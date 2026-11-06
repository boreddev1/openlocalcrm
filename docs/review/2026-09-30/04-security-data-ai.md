# 04 – Sicherheit B: Daten, Netzwerk, AI/PII, Audit

**Stand:** `main` @ `c6bac3e`, Review am 2026-09-30, read-only (kein Produktcode geändert).
**Grundlage:** Plan `docs/superpowers/plans/2026-09-30-full-source-review-plan.md` Task 4, `AGENTS.md`, Specs SP8b und SP8c, `docs/AI_COPILOT_AND_COMPLIANCE.md`, `docs/SECURITY_REVIEW_2026-09-21_round3.md`.
**Methode:** Statische Analyse mit `datei:zeile`-Belegen. Nicht live gegen PostgreSQL oder Docker verifiziert (kein `make up`). Befunde, deren Laufzeitverhalten nur aus dem Code abgeleitet ist, sind als „Live-Nachweis offen“ markiert.
**Severity:** S1 kritisch (Datenleck, Auth-Bypass, Datenverlust) · S2 hoch · S3 mittel · S4 niedrig/Hygiene. **Aufwand:** S < 0,5 PT · M ≤ 2 PT · L > 2 PT.
Querverweise auf `03-security-auth.md`: F-3-01 (E-Mail-IDOR), F-3-02 (SSE-Leak), F-3-04 (Master-Key), F-3-08 (Mail-Credential-Exfiltration).

---

## 1. Zusammenfassung

| ID | Sev | Kurztitel | Aufwand |
|---|---|---|---|
| F-4-01 | **S1** | Backups seit SP8c nicht wiederherstellbar: Der Export enthält `audit_logs.prev_hash/hash` und `ai_audit_logs.user_id`, die Restore-Allowlist lehnt diese Spalten ab | S |
| F-4-02 | S2 | Die Audit-Hash-Chain ist als Integritätsnachweis unwirksam (Race, JSONB-Normalisierung, Skip-Lücke, kein Aufrufer, kein Backup) | M |
| F-4-03 | S2 | Der PII-Guard erkennt weder Namen, E-Mail, Telefon, Adresse noch Geburtsdatum; bei externem Provider fließen CRM-Daten ungefiltert ab. Der PII-Zähler ist hart auf `false` gesetzt | M |
| F-4-04 | S2 | Notizen und Termine fallen bei DB-Fehler still auf In-Memory zurück → 201/200 ohne Persistenz (Datenverlust) | S |
| F-4-05 | S2 | IMAP-Sync lädt alle neuen Mails in den RAM, ohne Timeouts; eine defekte Mail blockiert den Sync dauerhaft | M |
| F-4-06 | S2 (bedingt) | Caddy ohne Hostnamen (`:80, :443`) → kein Auto-TLS; `DOMAIN` wird nicht genutzt; das Launcher-Template weicht ab | S |
| F-4-07 | S3 | SSRF über IMAP/SMTP-Hosts (auch BENUTZER) und Automations-Webhooks; ein SSRF-Guard existiert nur in `research.go` | M |
| F-4-08 | S3 | Audit-Lücken: CSV-Export, Mail-Versand und Mail-Konten, Notizen, Termine, Anrufe, KB und Freigaben nicht geloggt; CSV-Import-Akteur immer NULL; Connector-System-User existiert nicht | M |
| F-4-09 | S3 | Prompt-Injection-Härtung (SP8c) unvollständig: Triage (externe Mails!), Research, ParseBill und Drafter ohne Marker-Sanitizing | S |
| F-4-10 | S3 | Rohe `err.Error()` in API-Antworten (SP8b F-27 unvollständig), teils ungültiges JSON | S |
| F-4-11 | S3 | CSV-Export bricht still bei 10 000 Kontakten ab (Art. 20); SP8b-Import-Fehlerreport fehlt | S |
| F-4-12 | S3 | Das Backup enthält alle Secrets (Hashes, verschlüsselte TOTP- und Mail-Secrets) unverschlüsselt, aber nicht den Master-Key | M |
| F-4-13 | S3 | PII (Namen, Adressen) im Audit-Log, das wegen der Hash-Chain nicht löschbar ist (Konflikt mit Art. 17) | M |
| F-4-14 | S3 | Automation-Freigabe ohne Akteur und nicht atomar → Doppel-Ausführung von Webhooks und Entwürfen | S |
| F-4-15 | S4 | Storage-Layer nicht erreichbar, tote Body-Limit-Präfixe, 64-bit-Hash, Verzeichnisrechte 0755 | S |
| F-4-16 | S4 | AI-/Research-Responses ohne Größenlimit, Research erlaubt beliebige Ports auf öffentlichen Hosts | S |
| F-4-17 | S4 | `DEMO_MODE=1` aktiviert Demo-Ingest in einer Produktions-DB (inkonsistentes Parsing) | S |
| F-4-18 | S4 | SSE ohne Verbindungslimit pro User; zwei divergente CSP-Quellen (App vs. Caddy) | S |

---

## 2. Findings

### F-4-01 · S1 · Backups seit Migration 00021 nicht wiederherstellbar

- **Evidenz:**
  - Der Export streamt `SELECT row_to_json(t) FROM audit_logs t` (`internal/db/queries/backup.sql:275-276`, genutzt von `internal/db/backup_stream.go:158-160`). `row_to_json` liefert **alle** Spalten, also seit `internal/db/migrations/00021_audit_hash_chain.sql:3-8` auch `prev_hash`, `hash` sowie `ai_audit_logs.user_id`.
  - Die Restore-Allowlists kennen diese Spalten nicht: `internal/core/backup/tables.go:379-382` (`auditLogColumns`) und `:451-454` (`aiAuditLogColumns`).
  - Die Validierung bricht bei unbekannten Spalten hart ab: `internal/core/backup/restore.go:340-351` (`table %q does not have a column %q` → `ErrInvalidBackup`), und zwar vor jeder DB-Operation (`restore.go:57-61`).
  - Die Upsert-Statements schreiben `prev_hash`/`hash` ebenfalls nicht (`backup.sql:278-285`).
- **Impact:** Jede Instanz hat Audit-Zeilen (jeder Login schreibt welche, `internal/auth/service.go:323-327`). Damit ist jedes seit SP8c erzeugte Backup über `POST /backup/restore` nicht einspielbar: Datenverlust im Katastrophenfall. Der Drill streamt direkt Tabelle zu Scratch-Schema, ohne den Datei-Validator (`internal/core/backup/scratch.go:92ff`), und meldet deshalb vermutlich „bestanden“. Das ist eine falsche Sicherheit. Selbst nach einem Allowlist-Fix gehen die Chain-Spalten beim Restore verloren. Das verletzt die SP8a-§8-Regel „every new column is added to the backup contract“.
- **Fix:** `prev_hash`, `hash` (audit_logs) und `user_id` (ai_audit_logs) in Allowlist, Upsert und Demo-Querier aufnehmen. Regressionstest: Export → Restore(replace) über die echte Datei-Pipeline auf einer Live-DB. Einen Schema-Drift-Test ergänzen, der Allowlists gegen `information_schema.columns` prüft.
- **Live-Nachweis offen:** über `scripts/test-backup-integration.sh` bestätigen.
- **Aufwand:** S

### F-4-02 · S2 · Audit-Hash-Chain als Integritätsnachweis unwirksam

- **Evidenz:**
  1. **Race:** Der Vorgänger-Hash wird gelesen (`internal/core/audit/service.go:179-188`, `internal/db/queries/audit.sql:20-24`) und danach separat inseriert (`service.go:208-224,241-257`), ohne Advisory-Lock, SERIALIZABLE oder Unique-Constraint auf `prev_hash`. Parallele Requests erzeugen Verzweigungen, die spätere Verifikation meldet Manipulation.
  2. **JSONB:** `changes`, `old_values`, `new_values` sind `JSONB` (`00002_core_crm_schema.sql:93`, `00017_audit_logs_diff.sql:3-4`). Gehasht werden die Go-Bytes (`service.go:209,242`), verifiziert wird über die von PostgreSQL re-serialisierte JSONB-Darstellung (`service.go:294-305`). JSONB normalisiert Whitespace und Key-Reihenfolge, die Verifikation schlägt gegen echtes PostgreSQL daher voraussichtlich schon in Zeile 1 fehl. Die Tests laufen nur gegen den In-Memory-Querier (`service_test.go:218-272`, kein `TEST_DATABASE_URL`-Test für audit).
  3. **Skip-Lücke:** Zeilen mit leerem `prev_hash` werden übersprungen und ihr `hash` übernommen (`service.go:273-279`). Ein Angreifer mit DB-Zugriff setzt bei einer manipulierten Zeile `prev_hash=NULL` und belässt `hash`; die Kette verifiziert trotzdem.
  4. `created_at` und `id` sind nicht Teil des Hashes (`service.go:153-177`). Die Sortierung bei Zeitstempel-Gleichstand erfolgt nach zufälliger UUID (`audit.sql:23,28`).
  5. `VerifyAuditChain` hat in Produktion **keinen Aufrufer** (grep: nur Tests), weder Endpoint noch Job.
  6. Die Chain-Spalten sind nicht im Backup (F-4-01).
- **Impact:** Das SP8c-Versprechen „kryptografisch manipulationssicheres Audit-Log“ ist nicht erfüllt. Es gibt keine Erkennung, dafür False Positives bei Last. Revisionssicherheit (DSGVO Art. 5 Abs. 2, Rechenschaft) ist nicht belegbar.
- **Fix:**
  - Insert in eine Transaktion mit `pg_advisory_xact_lock(<const>)` legen oder `prev_hash` per `INSERT … SELECT hash FROM audit_logs ORDER BY seq DESC LIMIT 1 FOR UPDATE` bestimmen. Eine monotone `BIGSERIAL seq`-Spalte als Ordnung einführen.
  - Den Hash über kanonisches JSON bilden (in der DB mit `jsonb::text` des gespeicherten Werts nachrechnen oder `changes` als `TEXT` speichern). `created_at` und `seq` einbeziehen.
  - Die Skip-Regel auf Zeilen **vor** der ersten Chain-Zeile beschränken.
  - Einen Admin-Endpoint oder Worker-Job für die Verifikation mit Alarm bauen, optional Anchoring (periodischer Head-Hash ins Log bzw. extern). Test gegen echtes PostgreSQL mit Parallel-Inserts.
- **Aufwand:** M

### F-4-03 · S2 · PII-Guard lückenhaft; externe Provider erhalten Klartext-PII

- **Evidenz:**
  - Guard-Umfang: `internal/ai/guard.go:8-22` deckt nur IBAN, Kreditkarte, Steuer-ID, USt-ID und `key=secret` ab. Keine Regeln für E-Mail, Telefon (DE), Name, Straße/PLZ oder Geburtsdatum.
  - IBAN-Regex nur für Großbuchstaben und ASCII-Leerzeichen (`guard.go:10`) → `de89 3704 …`, NBSP oder Bindestriche umgehen ihn. Ländercodes mit alphanumerischer BBAN (z. B. GB, NL) werden nicht erkannt.
  - Der Guard wird zentral angewendet (`internal/ai/gateway.go:157-160,295-296`), bei `AI_PROVIDER=openai` gehen die Daten an `api.openai.com` (`gateway.go:176-180`).
  - Betroffene Prompts:
    - Triage mit vollständigem externen Mail-Body inklusive Signaturen (`internal/ai/triage.go:110-114`)
    - Notiz-Synthese mit Autor-E-Mail und Kundennotizen (`internal/server/handlers/notes.go:238-257`)
    - ParseBill mit Rechnungstext, Name und Zählernummer (`internal/server/handlers/ai.go:200-214`)
    - Drafter mit Empfängername (`internal/core/automation/draft.go:102-104`)
    - KB-Embeddings (`ai.go:308`)
  - Der Compliance-Zähler ist hart kodiert: `PIIFilterTriggered: false` in `internal/ai/chat.go:125,213,226`, `triage.go:126`, `research.go:197`. Die Redaktionsanzahl aus `SanitizeInputWithCount` (`guard.go:31`) wird verworfen (`gateway.go:159`).
- **Impact:** `AGENTS.md` verlangt: „PII must pass the guard before reaching any external LLM“. Bei externem Provider werden Namen, E-Mails, Telefonnummern und Adressen deutscher Endkunden an einen Drittland-Anbieter übermittelt (DSGVO Art. 44 ff.). Die Observability-Anzeige „PII-Filter ausgelöst“ ist irreführend (immer 0). Beim Default Ollama (lokal) ist das Risiko gering, daher S2 statt S1.
- **Fix:**
  - Regeln für E-Mail, DE-Telefon (+49/0…), PLZ + Straße, Datum (Geburtsdatum-Kontext) ergänzen. Case-insensitive, Unicode-Leerzeichen normalisieren.
  - Optional Pseudonymisierung bekannter CRM-Entitäten (Kontaktnamen aus DB → Platzhalter, Rückmapping).
  - Für externe Provider einen Opt-in-Schalter pro Feature einführen. Redaktionsanzahl aus dem Gateway zurückgeben und in `ai_audit_logs` schreiben.
  - Tabellengetriebene Guard-Tests mit Umgehungsvarianten.
- **Aufwand:** M

### F-4-04 · S2 · Stiller In-Memory-Fallback bei DB-Fehlern (Notizen, Termine)

- **Evidenz:**
  - `internal/core/note/service.go:190-231`: Liefert `CreateNote` einen Fehler, wird eine Notiz mit Fake-ID `not-xxxxxxxx` nur im Prozessspeicher angelegt, per SSE verteilt und mit 201 bestätigt. `Update` fällt ebenfalls durch (`:234-273`).
  - `internal/core/appointment/service.go:235-306` (Create) und `:314-350` (Update: DB-Fehler → In-Memory-„Update“, sogar Append eines neuen Eintrags, 200).
  - Reale Auslöser: `notes.author VARCHAR(100)` (`00008_notes_schema.sql:7`) bei langen E-Mail-Adressen (`handlers/notes.go:102-104`), FK-Verletzung bei unbekannter `contact_id`/`deal_id` in Terminen (`00006_…:6-8`), DB-Ausfall.
- **Impact:** Der Nutzer sieht „gespeichert“, die Daten sind nach einem Neustart weg: stiller Datenverlust, genau die in Round 3 bekämpfte Klasse „Feature lies about success“. Unbegrenztes Wachstum des Prozessspeichers (RAM-Budget). Phantom-Objekte werden an alle SSE-Clients verteilt.
- **Fix:** Im Produktionsmodus (querier ≠ nil) Fehler zurückgeben, nie auf In-Memory fallen. Die In-Memory-Pfade nur für den Querier-losen Demo-Modus zulassen. Validierung (Autor-Länge, UUIDs) vor dem Insert.
- **Aufwand:** S

### F-4-05 · S2 · IMAP/SMTP: unbegrenzter Speicher, keine Timeouts, blockierender Sync

- **Evidenz:**
  - `internal/mailclient/imap.go:125-133`: `Fetch(UID sinceUID+1:*)` mit vollem Body, danach `Collect()`. Alle Nachrichten werden gleichzeitig im RAM gepuffert, ohne Batch- oder Größenlimit.
  - `imap.go:65-73`, `smtp.go:72-85` (`smtp.Dial`, `tls.Dial`) ohne Dial-, IO- oder Deadline-Timeouts.
  - `imap.go:142-146`: Eine nicht parsebare Mail führt zu `return error`, der Watermark (`LastUID`) wird nicht fortgeschrieben (`internal/core/email/service.go:299-301`). Jeder weitere Sync scheitert an derselben Mail.
- **Impact:** Die Erst-Synchronisation eines realen Postfachs (Tausende Mails, Anhänge) sprengt das 420-MB-Budget → OOM des Servers oder Workers. Ein langsamer oder böswilliger Mailserver hält Goroutinen und Requests unbegrenzt fest. Eine einzige kaputte Mail stoppt den Posteingang dauerhaft.
- **Fix:** Fetch in UID-Batches (z. B. 50) mit `RFC822.SIZE`-Vorabprüfung und Maximalgröße. Pro Nachricht streamen. `net.Dialer{Timeout}` sowie `SetDeadline` bzw. `context` für IMAP und SMTP setzen. Unparsebare Nachrichten überspringen und protokollieren, den Watermark weiterschreiben.
- **Aufwand:** M

### F-4-06 · S2 (bedingt) · TLS-Terminierung: Caddy ohne Hostnamen

- **Evidenz:**
  - `Caddyfile:6` (`:80, :443 {`) enthält keinen Hostnamen. `DOMAIN` wird weder im Caddyfile noch im Compose an Caddy übergeben (`docker-compose.yml:4-18`). Auto-HTTPS (ACME) benötigt einen Hostnamen.
  - Das Launcher-Template schreibt eine abweichende Caddy-Konfiguration: ohne HSTS, ohne `header_up -X-Real-IP`, CSP mit `connect-src … http://localhost:8080 http://localhost:11434` (`internal/launcher/templates.go:117-151`). Dazu kommt ein Compose mit Default-DB-Passwort `crm_pass` (`templates.go:39,86`).
  - Die App-HSTS hängt an `X-Forwarded-Proto` (`internal/server/router.go:111-113`).
- **Impact:** Standardinstallationen liefern die App voraussichtlich nur über HTTP:80 aus. Passwörter, TOTP-Codes und Session-Cookies gehen dann im Klartext über das Netz (Cookies ohne `Secure`, `handlers/auth.go:16-27`). Das widerspricht der Stack-Angabe „Caddy v2 (Auto-TLS)“. Über den Launcher installierte Instanzen haben schwächere Header und eine schwache DB-Default-Credential (DB-Port nicht exponiert).
- **Fix:** Caddyfile auf `{$DOMAIN} { … }` mit Fallback `tls internal` für LAN umstellen, `DOMAIN` an den Caddy-Container durchreichen. Launcher-Template aus derselben Quelle generieren (Single Source, z. B. `//go:embed Caddyfile`), `crm_pass` durch Pflicht-Secret ersetzen.
- **Live-Nachweis offen:** Caddy-Verhalten im Deployment prüfen (`curl -vk https://<host>`).
- **Aufwand:** S

### F-4-07 · S3 · SSRF über Mail-Hosts und Automations-Webhooks

- **Evidenz:**
  - Jeder Nutzer mit `contacts.read`, also auch BENUTZER, kann einen persönlichen Mail-Account mit beliebigem `imap_host`/`smtp_host`/Port anlegen (`internal/server/handlers/emails.go:165-210`) und mit `/test` oder `/sync` Verbindungen auslösen (`:356-451`).
  - Die Fehlermeldung inklusive Netzfehler geht an den Client (`emails.go:394-397,443-446`) → Port-Scanner und Banner-Orakel für `db:5432`, `server:8080`, `169.254.169.254`, Hosts im LAN.
  - Automation-`WEBHOOK` postet an eine beliebige URL mit `http.Client{Timeout:5s}`, folgt Redirects und hat keinen IP-Guard (`internal/core/automation/service.go:522-552`). Die URL ist nur von ADMIN konfigurierbar, ausgelöst wird durch Events oder Test-Runs (VERTRIEB).
  - SP8b §2.6 verspricht Validierung „in `research.go` and `gateway.go`“. Umgesetzt ist sie nur in `internal/ai/research.go:100-150`.
- **Impact:** Interne Netzaufklärung aus dem Container-Netz heraus durch Nutzer mit niedrigster Rolle. Webhooks können interne Dienste (Ollama-Admin-API, Metadaten) ansprechen.
- **Fix:** Den vorhandenen `IsPrivateOrReservedIP`-Dialer (`research.go:42-99,111-126`) als gemeinsames Paket (`internal/netguard`) auslagern und für IMAP-, SMTP- und Webhook-Dial nutzen, mit Redirect-Check. Mail-Ports auf 25/465/587/143/993 beschränken. Fehlermeldungen generisch halten (F-4-10).
- **Aufwand:** M

### F-4-08 · S3 · Audit-Abdeckung und Akteur-Zuordnung lückenhaft

- **Evidenz:**
  - Audit wird nur von deal, contact, todo, invite, backup, company, auth, users, connectors, ownership und settings geschrieben (grep `audit.Log|LogDiff`).
  - **Nicht geloggt:**
    - CSV-Export aller Kontakte (`internal/server/handlers/export.go:21-33`, Massen-PII-Abfluss)
    - Mail-Versand und Mail-Konto-CRUD inklusive Credential- und Host-Änderung. `email.Service` hat ein `audit`-Feld, nutzt es aber nie (`internal/core/email/service.go:38,47`).
    - Notizen und Termine (Service ohne Audit)
    - Anrufe: `call_activities` ohne Nutzer-Spalte (`internal/core/telephony/service.go:56-61`)
    - KB-Löschung (`handlers/ai.go:411-439`)
    - Automation-Freigaben ohne Akteur (`handlers/automations.go:64-79`)
    - KI-Firmenanlage aus dem Chat ohne Owner und Audit (`internal/ai/chat.go:395-400`)
    - Owner-Wechsel (F-3-14)
  - **CSV-Import:** `actorID.Scan(claims.UserID)` übergibt ein `uuid.UUID` (Array). `pgtype.UUID.Scan` akzeptiert nur `string` (Modul-Quelle `pgtype/uuid.go:73-89`) → `actorID` ist **immer ungültig** (`export.go:36-40`). Importierte Kontakte sind unowned, ihre Audit-Einträge haben keinen Akteur.
  - **Connector:** Akteur `00000000-0000-0000-0000-000000000001` (`internal/server/handlers/connectors.go:32-35`, `internal/connectors/connector.go:76-77`) existiert nicht in `users`. `audit_logs.user_id REFERENCES users(id)` (`00002_core_crm_schema.sql:89`) → FK-Verletzung. Die Audit-Zeilen für Lead-Intake (SP8b §2.7) und das initiale `SetContactOwner` (`contact/service.go:302-311`) schlagen still fehl (Fehler ignoriert).
- **Impact:** Keine Rechenschaft über Massenexporte, Postfach-Zugriffe und Credential-Änderungen (Art. 5 Abs. 2, Art. 30/32 DSGVO). Importierte Datensätze sind für BENUTZER unsichtbar und nicht zuordenbar.
- **Fix:**
  - Audit in allen genannten Pfaden ergänzen. Für den Export IP, User-Agent und Zeilenzahl loggen.
  - `pgtype.UUID{Bytes: claims.UserID, Valid: true}` statt `Scan`.
  - Einen echten System-User per Migration anlegen oder `user_id` NULL plus `actor='system:connector'` in `changes`.
  - Audit-Fehler mindestens loggen oder metrisieren.
- **Aufwand:** M

### F-4-09 · S3 · Prompt-Injection-Härtung unvollständig

- **Evidenz:**
  - `SanitizeRoleMarkers` wird nur in `internal/ai/chat.go:109,199` und `handlers/notes.go:243` verwendet.
  - **Nicht verwendet:** Triage mit externem Mail-Body (`internal/ai/triage.go:110-114`, das schließende `</incoming_email>` wird nicht neutralisiert), Research-Titel und Meta (`research.go:176-185`, außerhalb des Untrusted-Blocks), ParseBill (`handlers/ai.go:200-212`), Drafter (`automation/draft.go:102`).
  - Ersetzungen sind case-sensitive (`chat.go:68-84`, z. B. `[inst]` oder `<|IM_START|>` bleiben stehen).
  - Die Chat-Rolle kommt ungeprüft vom Client und wird als `SYSTEM:`-Präfix geschrieben (`chat.go:108-110`).
  - Der Output-Guard ersetzt nur `<script>`/`javascript:` (`guard.go:21,76-78`). Markdown-Bild-Exfiltration ist im Frontend nicht relevant, da kein Markdown/HTML gerendert wird (grep `dangerouslySetInnerHTML`/`react-markdown` ohne Treffer).
- **Impact:** Angreifer von außen (eingehende Mail, Webseite) können die Triage-Klassifikation und Research-Zusammenfassungen steuern. Das ist mittelbar relevant, da Triage-Ergebnisse Automationen und Priorisierung beeinflussen.
- **Fix:** Einheitlichen `ai.WrapUntrusted(tag, text)` bauen: Marker case-insensitive neutralisieren, alle schließenden Tags entfernen. In allen Prompt-Buildern nutzen. Rollen im Chat auf `user|assistant` whitelisten.
- **Aufwand:** S

### F-4-10 · S3 · Fehlerdetails in API-Antworten (SP8b F-27 unvollständig)

- **Evidenz:**
  - String-Konkatenation in JSON: `internal/server/handlers/emails.go:203,306` (ungültiges JSON bei Anführungszeichen, leakt DB- und Crypto-Fehler).
  - `err.Error()` im Body:
    - `emails.go:396,445,518` (Netz- und Serverantworten)
    - `users.go:159`
    - `backup.go:121,218,227`
    - `ai.go:115,136,161,310,376`
    - `export.go:56`
    - `internal/ai/chat.go:402` (DB-Fehler im Chat-Text)
- **Impact:** Offenlegung interner Hostnamen, Constraint- und Tabellennamen, Upstream-Antworten. Unterstützt F-4-07 als Orakel. SP8b §2.3 sagt: „Never string-concatenate `err.Error()`“.
- **Fix:** `writeJSONError` mit festen Codes verwenden, Details nur loggen (mit Request-ID). Lint-Regel oder Test, die `err.Error()` in Handler-Responses verbietet.
- **Aufwand:** S

### F-4-11 · S3 · CSV-Export und -Import unvollständig

- **Evidenz:**
  - `internal/core/export/service.go:42-45`: Hartes `Limit: 10000`, keine Pagination, kein Hinweis. Die ganze Datei wird im RAM aufgebaut (`:50-118`).
  - Import überspringt fehlerhafte Zeilen still (`:179-181,183-190,225-227`) und liefert nur `imported_count` (`handlers/export.go:61`). SP8b §2.8 verspricht `{imported, failed, errors[]}` mit Zeilennummern.
  - Formel-Injection-Schutz ist vorhanden (`service.go:29-38`).
- **Impact:** Ein Art.-15/20-Export bei mehr als 10 000 Kontakten ist unvollständig, ohne dass der Nutzer es merkt. Import-Fehler bleiben unsichtbar.
- **Fix:** Streaming-Export über `ListContacts` mit Cursor und `csv.Writer` direkt auf `ResponseWriter`. Import-Report gemäß Spec.
- **Aufwand:** S

### F-4-12 · S3 · Backup-Inhalt: Secrets unverschlüsselt, Master-Key fehlt

- **Evidenz:**
  - Exportierte Spalten: `password_hash`, `totp_secret_encrypted` (`internal/core/backup/tables.go:332-336`), `password_encrypted` der Mail-Konten (`:363-367`), `token_hash` der Refresh-Tokens (`:374-377`) und der Invites (`userInviteColumns`).
  - Download als Klartext-JSON (`handlers/backup.go:78-84`), keine Verschlüsselung.
  - Der AES-Master-Key (`internal/crypto/crypto.go:22-31`) ist nicht Teil des Backups.
- **Impact:** Eine abgelegte Backup-Datei ermöglicht Offline-Cracking aller Passwörter und enthält die komplette Kundendatenbank im Klartext. Umgekehrt ist ein Restore auf einem neuen Host für TOTP und Mail-Credentials wertlos (Lockout, siehe F-3-04).
- **Fix:** Backup optional mit Passphrase (age/AES-GCM) verschlüsseln. Master-Key-Sicherung separat dokumentieren (bzw. Key-Wrapping per Passphrase in das Backup aufnehmen). Refresh-Tokens aus dem Backup ausschließen (Sessions müssen nicht wiederhergestellt werden).
- **Aufwand:** M

### F-4-13 · S3 · Personenbezogene Daten im unlöschbaren Audit-Log

- **Evidenz:**
  - `internal/core/audit/service.go:78-105` maskiert nur Keys mit `phone`/`mobile`/`telefon`, `email`/`mail` sowie `password`/`secret`/`token`.
  - Namen, Adressen, Zählernummern und Verbräuche bleiben im Klartext, z. B. bei Contact-Create und -Update und bei `deal/service.go:152` (komplettes Deal-Objekt).
  - Die Hash-Chain (00021) macht jede nachträgliche Löschung oder Anonymisierung zum „Manipulationsfall“.
- **Impact:** Konflikt zwischen Art. 17 (Löschung beim Kontakt-Delete) und Revisionssicherheit. Das Audit-Log wird zur Schattenkopie der Kundendaten.
- **Fix:** Im Audit nur Feldnamen und Hash-Werte (oder pseudonymisierte IDs) statt Klartextwerten speichern. Alternativ Klartext in einer separaten, löschbaren Tabelle ablegen, die von der Chain nur per Hash referenziert wird.
- **Aufwand:** M

### F-4-14 · S3 · Automation-Freigabe ohne Akteur, nicht atomar

- **Evidenz:** `internal/server/handlers/automations.go:64-79` reicht keinen Nutzer weiter. `internal/core/automation/service.go:251-262` prüft `Status == "WAITING_APPROVAL"` per Read und führt dann Steps aus (`:293-312`), ohne atomaren Statuswechsel.
- **Impact:** Human-in-the-Loop ohne Nachweis, wer freigegeben hat. Zwei parallele Freigaben (Doppelklick, zwei Tabs) führen Webhook oder E-Mail-Entwurf doppelt aus.
- **Fix:** `UPDATE workflow_runs SET status='IN_PROGRESS', approved_by=$u WHERE id=$1 AND status='WAITING_APPROVAL' RETURNING …` als Claim. Akteur ins Snapshot und Audit schreiben.
- **Aufwand:** S

### F-4-15 · S4 · Storage-Layer und Body-Limits

- **Evidenz:**
  - `internal/storage/local.go` wird von keinem HTTP-Handler aufgerufen (grep `.Save(`/`.Open(` in `internal/server`, `internal/core` ohne Treffer). Das Body-Limit-Präfix `/api/v1/storage` (`router.go:125`) ist tot.
  - `/api/v1/import/contacts` bekommt 5 MB statt der in SP8b vorgesehenen 50 MB, `ParseMultipartForm(10<<20)` (`export.go:42`) ist daher irreführend.
  - Der CAS-Name nutzt nur 64 bit des SHA-256 (`local.go:88`), Verzeichnisse 0755 (`:57`).
  - Path-Sanitizing (`local.go:44-50`) und der MIME-Check vor dem Dedup (`:83-86`) sind korrekt.
- **Fix:** Tote Pfade entfernen oder Upload-Route bewusst ergänzen. Den vollen Hash verwenden, Rechte 0750.
- **Aufwand:** S

### F-4-16 · S4 · AI-/Research-Netzwerkhygiene

- **Evidenz:**
  - `internal/ai/gateway.go:234,278,377,429`: `json.NewDecoder(resp.Body)` ohne `io.LimitReader`.
  - `research.go:157-165`: Die Domain darf `host:port` enthalten, also beliebige Ports öffentlicher Hosts.
  - Der Guard prüft alle aufgelösten IPs, verbindet per IP und prüft Redirects (`research.go:111-150`), DNS-Rebinding ist damit abgedeckt.
- **Fix:** Response-Limit (z. B. 4 MB). Port auf 443/80 beschränken.
- **Aufwand:** S

### F-4-17 · S4 · Inkonsistentes DEMO_MODE-Parsing

- **Evidenz:** `cmd/server/main.go:159` (`EqualFold(..., "true")`) gegenüber `internal/server/handlers/emails.go:25,81` (akzeptiert auch `"1"`).
- **Impact:** Mit `DEMO_MODE=1` läuft der Server im Produktionsmodus mit PostgreSQL, `POST /emails/demo-ingest` ist aber freigeschaltet. Jeder Nutzer kann Fake-Mails in den echten Posteingang schreiben, die Automationen triggern.
- **Fix:** Nur das `cfg.DemoMode`-Flag aus dem Router verwenden, keinen eigenen Env-Fallback.
- **Aufwand:** S

### F-4-18 · S4 · SSE-Limits und CSP-Dopplung

- **Evidenz:**
  - `internal/sse/hub.go:71-77`: Kein Limit pro User. Jeder Stream hält Goroutine und Puffer (16).
  - CSP wird von der App (`router.go:110`, `script-src 'self' 'unsafe-inline'`) und von Caddy (`Caddyfile:22`, `script-src 'self'`) gesetzt. Beim Proxying kommen voraussichtlich zwei CSP-Header an. Das ist restriktiv, aber schwer wartbar und die App-CSP ist schwächer.
- **Fix:** Maximal N Streams pro User. Eine CSP-Quelle (App), `'unsafe-inline'` für Scripts entfernen, sofern der Vite-Build es erlaubt.
- **Aufwand:** S

---

## 3. AI-Aufrufer-Inventar (Task 4 Step 1)

| Pfad | Aufruf | Guard (PII) | Role-Marker | Beleg |
|---|---|---|---|---|
| Chat | `gateway.Generate` | ja (Gateway) | ja | `internal/ai/chat.go:109,199,203` |
| Chat-Firmenanlage | DB-Write ohne LLM | – | – | `chat.go:395-400` (kein Owner, kein Audit) |
| Triage | `gateway.Generate` | ja | **nein** | `triage.go:110-116` |
| Research | `gateway.Generate` | ja | **nein** (Titel/Meta) | `research.go:176-187` |
| Notiz-Synthese | `gateway.Generate` | ja | ja | `handlers/notes.go:243-259` |
| ParseBill | `gateway.Generate` | ja | **nein** | `handlers/ai.go:200-214` |
| KB-Create/Search | `GenerateEmbedding` | ja | – | `handlers/ai.go:308,374`, `gateway.go:295-296` |
| E-Mail-Template-Generator | `Generate` über `*ai.Gateway` | ja | nein | `core/emailtemplate/service.go:265`, `router.go:242` |
| AI-Drafter (Automation) | `Generate` über `*ai.Gateway` | ja | nein | `core/automation/draft.go:104`, `router.go:232` |

Ergebnis: Kein Provider-Aufruf umgeht das Gateway, alle direkten HTTP-Clients sind in `gateway.go`, `research.go`, `automation/service.go` oder im Launcher (grep `http.(Get|Post|NewRequest…)`). Die Lücke liegt im **Umfang** des Guards (F-4-03), nicht in seiner Umgehung.

## 4. Ausgehende Requests (Task 4 Step 2)

| Ziel | Guard | Timeout | Größenlimit | Beleg |
|---|---|---|---|---|
| Research-Scraper | IP-Guard, Dial per IP, Redirect-Check | 1 s / 800 ms | 256 KB | `research.go:108-150,241-253` ✔ |
| LLM (Ollama/OpenAI) | keiner (Admin-Config) | 2 s Dial, 30 s gesamt | **keins** | `gateway.go:140-149` (F-4-16) |
| Webhook | **keiner** | 5 s | Body nicht gelesen | `automation/service.go:528-552` (F-4-07) |
| IMAP/SMTP | **keiner** | **keiner** | **keins** | `mailclient/*.go` (F-4-05, F-4-07) |
| Geocoding | kein Netz (Offline-Daten) | – | – | `internal/geocoding/` ohne `net/http` ✔ |
| Launcher-Updater | Checksums (Task 8) | – | – | `internal/launcher/updater.go` (Task 8) |

## 5. Verifiziert OK (mit Evidenz)

- **SQL-Injection:** Alle Fach-Queries laufen über sqlc (`internal/db/queries/*.sql`). Dynamisches SQL nur im Backup-Drill, dort mit `pgx.Identifier{…}.Sanitize()` und validiertem Schemanamen (`internal/core/backup/scratch.go:278,286-292,408-424`). Restore nutzt statische Upserts ohne Identifier aus der Datei (`tables.go:14-17`). Kein `fmt.Sprintf` mit SELECT/WHERE in `internal/server` oder `internal/core`.
- **CSV-Formel-Injection:** Präfix `'` für `= + - @ \t \r` (`internal/core/export/service.go:29-38`), angewendet auf alle Textfelder (`:96-114`).
- **Frontend-Ausgabe:** Keine HTML-Sinks (`dangerouslySetInnerHTML`, `innerHTML`, `srcDoc`, `react-markdown`) in `web/src`. `body_html` wird nicht gerendert (nur als Typ in `web/src/api/client.ts:413`). `href` nur `tel:` und die interne ICS-URL.
- **ICS:** Felder escaped (`internal/core/appointment/service.go:384-392`), Content-Type `text/calendar`.
- **Storage:** Path-Traversal-Schutz (`local.go:44-50`), MIME-Allowlist mit Magic Bytes und HTML/SVG/Script-Block vor dem Dedup (`:83-86,109-142`). SP8b §2.2 ist umgesetzt, aber nicht erreichbar (F-4-15).
- **HTTP-Härtung App:** `nosniff`, `X-Frame-Options DENY`, Referrer- und Permissions-Policy, CSP, HSTS hinter TLS (`router.go:103-116`). Body-Limits 5/50/100 MB (`:118-132`). Server-Timeouts Read 15 s und Write 30 s (`cmd/server/main.go:229-234`), SSE und Backup-Export setzen die Deadline zurück (`sse/hub.go:103-105`, `handlers/backup.go:75-76`). Health ohne AI-Config (`router.go:134-143`). Kein CORS-Header, also nur Same-Origin.
- **Container:** Kein Host-Port für server, worker oder db (`docker-compose.yml:21-94`, SP8b F-42). Non-root `USER 10001` (`Dockerfile.server`, `Dockerfile.worker`). `DB_PASSWORD` ist Pflicht (`docker-compose.yml:79`). Caddy entfernt `True-Client-IP` und `X-Real-IP` (`Caddyfile:30-48`).
- **Backup-Endpunkte:** ADMIN, Rate-Limit 10/min (`router.go:418-424`), Restore-Größe 100 MB (`handlers/backup.go:153`). Replace-Modus nur mit vollständigem Tabellenumfang (`restore.go:67-72`). Aussage zu `committed` ist ehrlich (`backup.go:198-213`).
- **Connector:** Token-Vergleich in konstanter Zeit, leeres Token wird abgelehnt (`internal/connectors/connector.go:68`). Rate-Limit 60/min (`router.go:345`). Demo-Token nur im Demo-Modus (`router.go:235-238`).
- **Mail-Transport:** TLS-Pflicht, kein Plaintext-AUTH (`internal/mailclient/smtp.go:90-100`). IMAP nur TLS oder STARTTLS (`imap.go:69-72`). `InsecureSkipVerify` wird nur in Tests gesetzt.
- **Research-SSRF:** Private, reservierte, CGNAT-, Metadata- und Multicast-Bereiche einschließlich IPv4-mapped IPv6 werden geblockt (`research.go:42-99`). Verbindung per aufgelöster IP (gegen Rebinding, `:111-126`), Redirect-Check (`:131-146`).
- **AI-Attribution (SP8c):** `user_id` wird aus den Request-Claims übernommen (`internal/ai/observability.go:62-66,77-82`). Migration `00021_audit_hash_chain.sql:7-8`.
- **Security-Events (SP8c):** LOGIN_FAILED/SUCCESS, REFRESH_TOKEN_REUSE, LOGOUT, TOTP_*, USER_ROLE/STATUS_CHANGED, SETTINGS_*, BULK_ASSIGN_OWNER, BACKUP-Operationen werden geschrieben (`auth/service.go:199-327,425-433`, `users.go:245-252,319-326`, `settings.go:99-106,149-155`, `ownership_admin.go:98-121`).

## 6. Abgleich früher gemeldeter Fixes (Round 3, SP8b, SP8c – Daten/AI-Teil)

| Früherer Punkt | Ergebnis | Evidenz |
|---|---|---|
| R3 N3 Appointment-Delete false success | **behoben** | `handlers/appointments.go:173-176` |
| R3 N4 ApproveStep-Fehler verschluckt | **behoben** (neu: Race/Akteur F-4-14) | `automation/service.go:298-306,327-335` |
| R3 N5 Webhook No-Op | **implementiert, ohne SSRF-Guard** | F-4-07 |
| R3 N6 ParseBill Fake | **behoben** (echter LLM-Aufruf) | `handlers/ai.go:180-238` |
| R3 N7 DeleteKB DB-Fehler | **behoben** | `handlers/ai.go:419-423` |
| R3 N12 CSV-Import ohne Backend | **behoben** (Akteur-Bug F-4-08) | `web/src/components/contacts/CSVImportModal.tsx:71-72` |
| R3 N13 Fake-Vektorisierung | **behoben** (echte Embeddings, pgvector) | `handlers/ai.go:308-326,374-389` |
| R3 N10/N14 PushExternal | **entfernt** (keine Route) | grep `PushExternal` ohne Treffer |
| R3 H3 Chat schreibt ohne Bestätigung | **behoben** (Bestätigungsschritt) | `internal/ai/chat.go:356-362` |
| R3 M5 stiller Gateway-Fallback | **behoben** | `gateway.go:218-224,266-273` liefern `ErrUpstreamUnavailable` |
| R3 M6 E-Mail-Worker-Stubs | **Code echt** (`queue/jobs.go:130`), **scheitert aber an Key-Divergenz** | F-3-04 |
| SP8b §2.2 Upload-Allowlist, MIME vor Dedup | **bestätigt**, aber unerreichbar | `storage/local.go:20-27,83-86`; F-4-15 |
| SP8b §2.3 Fehler-Sanitizing | **unvollständig** | F-4-10 |
| SP8b §2.3 Health ohne AI-Leak | **bestätigt** | `router.go:134-143` |
| SP8b §2.4 SSE per-User für Notifications | **bestätigt** für Notifications; andere Broadcasts leaken | `notification/service.go:62-66`; F-3-02 |
| SP8b §2.5 Timeouts und Body-Limits | **bestätigt** (Upload-Pfad tot) | `main.go:229-234`, `router.go:118-132` |
| SP8b §2.6 SSRF research + gateway | **nur research** | F-4-07, F-4-16 |
| SP8b §2.6 Ports nur via Proxy | **bestätigt** | `docker-compose.yml` |
| SP8b §2.7 Lead-Intake: Rate-Limit, Token-Pflicht | **bestätigt** (Antwort 401 statt spezifiziert 503) | `router.go:345`, `connector.go:68` |
| SP8b §2.7 Lead-Intake-Audit | **wirkungslos** (FK auf nicht existenten System-User) | F-4-08 |
| SP8b §2.8 CSV-Header-Validierung | **bestätigt** | `export/service.go:157-160` |
| SP8b §2.8 Zeilen-Fehlerreport | **nicht umgesetzt** | F-4-11 |
| SP8c Audit-Hash-Chain | **vorhanden, aber unwirksam und bricht Backups** | F-4-02, F-4-01 |
| SP8c PII-Regex-Verfeinerung (Steuer-ID mit Kontext) | **bestätigt**, Abdeckung aber lückenhaft | `guard.go:13-15`; F-4-03 |
| SP8c Role-Marker-Sanitizing | **teilweise** (Chat, Notizen) | F-4-09 |
| SP8c AI-User-Attribution | **bestätigt** | `observability.go:62-66` |
