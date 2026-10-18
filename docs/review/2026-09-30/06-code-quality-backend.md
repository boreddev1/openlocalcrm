# Review 06 – Code-Qualität, Robustheit, Performance (Backend)

Stand: 2026-09-30, Commit `c6bac3e`. Read-only-Review (Task 6 des Review-Plans). Kein Produktcode geändert.
Severity: S1 kritisch, S2 hoch, S3 mittel, S4 niedrig. Aufwand: S/M/L.

Nicht gemessen: RAM unter Last (`docker stats` nicht erlaubt), Laufzeitverhalten gegen echte IMAP-/Postgres-Instanzen. Alle Aussagen beruhen auf Code-Lektüre; das ist jeweils vermerkt. Der `golangci-lint`-Lauf war nicht auswertbar (F-6-16).

## Step 1 – Struktur & Größe

Größte Nicht-Test-Dateien (`find ... | xargs wc -l`, gesamt 28 068 Zeilen): `internal/db/demo/querier.go` 2898, `internal/launcher/updater.go` 899, `internal/launcher/engine.go` 882, `cmd/setup-launcher/cli.go` 735, `internal/core/automation/service.go` 716, `internal/launcher/server.go` 690, `internal/server/router.go` 627, `internal/core/backup/scratch.go` 611, `internal/auth/service.go` 599, `internal/core/contact/service.go` 571, `internal/server/handlers/emails.go` 557, `internal/server/handlers/ai.go` 552.
Bewertung: > 500 Zeilen bei 12 Dateien; `demo/querier.go` (Reimplementierung des gesamten Querier im Speicher) ist der größte Wartungsposten, jede neue Query muss dort doppelt gepflegt werden (siehe F-6-15). Die `core/<domain>` + `handlers/<domain>.go`-Struktur wird eingehalten. `internal/server/csrf.go` ist ein 24-Zeilen-Wrapper, der an `internal/auth/csrf.go` delegiert (`server/csrf.go:14-24`), kein echtes Duplikat.

## Findings

### F-6-01 (S2) – SSE verteilt Notizen, Termine, E-Mail-Metadaten an alle angemeldeten Nutzer (Ownership umgangen)
- Evidenz: `internal/core/note/service.go:300-306` (`broadcast` -> `Hub.Broadcast`), aufgerufen mit der vollständigen Notiz `n` in `:210,230,252,269` (Inhalt der Notiz!); `internal/core/appointment/service.go:375-381` mit dem ganzen `Appointment` inkl. privater Termine (`:277,305,333`); `internal/core/email/service.go:153-163` Betreff, Absender, `contact_id` jeder eingehenden Mail, auch aus persönlichen Postfächern (`account_type personal`, `00013`); `internal/core/telephony/service.go:80-90`. Der Hub kennt Benutzer (`sse/hub.go:14-16`, `SendToUser` existiert `:47`), wird aber hier nicht genutzt; `Broadcast` adressiert jeden Client (`hub.go:28-39`).
- Auswirkung: Jeder eingeloggte `BENUTZER` kann über `/events/stream` Notizen, Termine und Mailmetadaten anderer Nutzer mitlesen, obwohl die REST-Endpunkte Ownership erzwingen. Umgeht SP8a-Ownership und ist DSGVO-relevant (Vertraulichkeit persönlicher Postfächer).
- Fix: Events mit `OwnerUserID` taggen und per `SendToUser` bzw. Rollen-/Team-Filter zustellen; für persönliche Postfächer nur an `owner_user_id`. Payload minimal halten (nur ID + Typ, Client lädt nach). Test: Nutzer B erhält keinen Event zu Daten von Nutzer A. Aufwand: M.

### F-6-02 (S2) – Inaktivitäts-Scan erzeugt stündlich wiederholt Aufgaben für dieselben Kontakte und erreicht nie die übrigen
- Evidenz: `internal/queue/client.go:47-53` (stündlich, `RunOnStart: true`), `internal/queue/jobs.go:185-196`: `ListInactiveContacts(updated_at < cutoff, LIMIT 100)` (`queries/contacts.sql:94-98`, `ORDER BY updated_at ASC`). Es gibt weder ein Merkmal "bereits getriggert" noch Deduplikation in `TriggerEvent` (`internal/core/automation/service.go:172-233`, Grep nach `dedup|already` leer); der Standard-Workflow `wf-3` (`00016`) führt `CREATE_TASK` aus (`service.go:408-431`, `CreateTodo` ohne Existenzprüfung). Die Aufgabe setzt `updated_at` des Kontakts nicht zurück.
- Auswirkung: Jede Stunde entstehen für dieselben bis zu 100 ältesten inaktiven Kontakte neue Todos und Workflow-Runs (bis 2400 Todos/Tag); Kontakte jenseits der ersten 100 werden nie verarbeitet. Nutzer werden mit Duplikaten geflutet, `workflow_runs` wächst unbegrenzt.
- Fix: Dedup über letzten Run je `(workflow, target)` (z. B. kein neuer Trigger innerhalb der SLA-Frist), Wasserzeichen/Cursor oder `NOT EXISTS`-Filter in der Query, Seitenweise Verarbeitung. Test für Wiederholung. Aufwand: M.

### F-6-03 (S2) – IMAP-Abruf: alles im Speicher, keine Timeouts, eine defekte Mail blockiert die Synchronisation dauerhaft
- Evidenz: `internal/mailclient/imap.go:131-134` `fetchCmd.Collect()` lädt alle neuen Nachrichten (`BodySection` komplett, inkl. Anhänge) in den RAM; `:190-193` `io.ReadAll(part.Body)` pro Teil ohne Limit; pro Nachricht zusätzlich `bytes.NewReader(raw)` (`:162`). `dialIMAP` (`:65-73`) ohne Dial-/IO-Timeout, `FetchNewMessages(cfg, sinceUID)` bekommt keinen `context` (`:98`), ist also nicht abbrechbar (Worker-Shutdown, River-Timeout). `:140-144`: Parsing-Fehler einer Nachricht gibt `FetchResult{}` + Fehler zurück, `LastUID` wird nicht fortgeschrieben (`core/email/service.go:295-298`).
- Auswirkung: (1) Erstsync eines großen Postfachs (tausende Mails, Anhänge) kann das 420-MB-Budget sprengen (OOM im Worker). (2) Hängender IMAP-Server blockiert einen River-Worker-Slot bzw. Goroutine unbegrenzt. (3) Eine einzige nicht parsebare Nachricht lässt jeden Sync-Versuch scheitern (Retry-Schleife, Mail danach nie abrufbar). Nicht gegen echten Server geprüft.
- Fix: UID-Bereiche in Batches (z. B. 50) abrufen, Größen-/Anhangslimit, `net.Dialer{Timeout}` + `imapclient.Options{Dialer/Timeout}` bzw. Kontext-gesteuertes Schließen, fehlerhafte Nachricht loggen und überspringen (UID trotzdem weiterschieben). Aufwand: M.

### F-6-04 (S2) – E-Mail-Sync kann Nachrichten stillschweigend verlieren
- Evidenz: `internal/core/email/service.go:313-316`: `if err != nil { continue }` ("duplicate" und jeder andere Fehler, auch transiente DB-Fehler), danach wird `LastUid` auf den höchsten UID gesetzt (`:320-325`). `email_messages.message_id VARCHAR(255) UNIQUE NOT NULL` ist global (`00003_email_schema.sql:34`), nicht je Konto. Fehlt im Header eine Message-ID, ist `fm.MessageID` leer (`imap.go:176-178`).
- Auswirkung: (a) Transienter DB-Fehler bei `IngestMessage` -> Mail geht verloren, UID wird trotzdem weitergeschoben. (b) Dieselbe Mail in zwei Konten (persönlich + Team) wird im zweiten Konto wegen globaler Eindeutigkeit verworfen. (c) Leere oder doppelte Message-IDs kollidieren.
- Fix: Nur Unique-Violation überspringen (`db.IsUniqueViolation` existiert), andere Fehler abbrechen bevor `LastUid` gesetzt wird; `UNIQUE (account_id, message_id)`; Fallback-ID aus Hash (Header+Datum) wenn leer. Aufwand: M.

### F-6-05 (S3) – Listen ohne DB-Limit bzw. Pagination im Speicher
- Siehe F-5-09 (vollständige Liste). Ergänzend Handler-Ebene: `internal/server/handlers/deals.go:116-119`, `appointments.go:38-41`, `contacts.go:122-127`, `todos.go:76-80`, `companies.go:86-90`, `emails.go:33-37` parsen `limit/offset` mit `strconv.Atoi` und verwerfen Fehler (`limit, _ :=`). `ListByStage` (`core/deal/service.go:196-227`) und `appointment.List` (`core/appointment/service.go:100-127`) laden alles, schneiden dann im Speicher (Clamp 500). `ai.go:360-366` (`SearchKB`) ignoriert ungültiges `limit` ohne Fehler, Maximum 50 ist vorhanden.
- Auswirkung: Ungültige Parameter führen nicht zu 400, sondern stillem Default; negativer `offset` wird an SQL weitergereicht (`core/contact/service.go:334-346` prüft ihn nicht) und endet als 500 (`OFFSET must not be negative`); `int32(limit)` kürzt sehr große Werte. Kein Sortier-Whitelist-Problem, weil Sortierung fest in SQL steht (verifiziert OK).
- Fix: Gemeinsamer Helfer `parsePage(r)` mit 400 bei Parse-Fehler, Clamp (Default 50, Max 100/500) und SQL-`LIMIT/OFFSET` überall. Aufwand: S–M.

### F-6-06 (S3) – Interne Fehlertexte in HTTP-Antworten; JSON per String-Konkatenation gebaut
- Evidenz (SP8b behauptet "sanitisiert", nicht für alle Handler wahr): `internal/server/handlers/emails.go:203` und `:306` (`"failed to create email account: "+err.Error()` direkt in JSON-String, nicht escaped -> ungültiges JSON bei Anführungszeichen, leakt DB-/Crypto-Details), `emails.go:396,445,518` (IMAP-/SMTP-Fehler inkl. Host/Adresse), `ai.go:115,136,161,310,376,502`, `email_templates.go:184-192`, `backup.go:121,218,227`, `export.go:56`, `users.go:159`, `auth.go:93`.
- Auswirkung: Information Disclosure (Hostnamen, SQL-/Treiberfehler, Provider-Antworten), inkonsistentes/ungültiges JSON. Bei IMAP/SMTP evtl. Serverbanner/Credentials-Hinweise.
- Fix: Sentinel-Fehler auf Nutzertexte abbilden, Rohfehler nur serverseitig loggen (mit Request-ID); einheitlicher JSON-Helfer (F-6-10). Aufwand: S–M.

### F-6-07 (S3) – River-Jobs: keine Eindeutigkeit, Retry-Standard, Shutdown ohne Timeout
- Evidenz: `internal/queue/jobs.go:95-101` und `:129-133` fügen pro Konto alle 60 s (`client.go:16`, `RunOnStart: true`) bzw. pro Mail neue Jobs ein, ohne `river.InsertOpts{UniqueOpts}`; `Queues: {default: MaxWorkers: 10}` (`client.go:34-36`), keine `MaxAttempts`/Timeouts/Dead-Letter-Konfiguration (River-Defaults: 25 Versuche mit Backoff). `cmd/worker/main.go:99`: `client.Stop(context.Background())` ohne Zeitlimit; Go-Fehler von `client.Start` endet in `log.Fatalf` (`:95`) ohne Drain.
- Auswirkung: Dauert ein Sync länger als 60 s (siehe F-6-03), laufen mehrere Syncs desselben Kontos parallel und produzieren doppelte Mails/Triage-Jobs (UID-Watermark wird erst am Ende gesetzt). Triage wiederholt sich bei jedem Sync erneut für dieselben Nachrichten, wenn die Ingestion fehlschlug. Stop kann bei hängendem Job unbegrenzt blockieren (Docker killt nach Grace-Period). Jobs sind sonst weitgehend idempotent (Tags überschreiben, UID-Watermark) – Verifiziert: `jobs.go:148-160`.
- Zusätzlich: Standard-DB-URL mit Klartext-Zugangsdaten als Fallback (`cmd/worker/main.go:36-38`, `crm_user:crm_pass`), `cmd/worker` hat keine Tests (`ls cmd/worker` -> nur `main.go`).
- Fix: `UniqueOpts{ByArgs: true, ByPeriod}` für Sync/Triage, `MaxAttempts` und `Timeout` setzen, `StopAndCancel` mit Deadline, Fallback-DSN entfernen (Fehler bei fehlender Variable, wie im Server prüfen). Aufwand: S–M.

### F-6-08 (S3) – SSE-Hub: keine Client-Obergrenze, kein Heartbeat, abgeschaltetes Write-Deadline
- Evidenz: `internal/sse/hub.go:66-73` (`register` ohne Limit), `:113` `SetWriteDeadline(time.Time{})`; kein periodischer Keep-Alive/Kommentarzeilen in der Schleife `:131-147`; Router vergibt pro Verbindung nur Token-Prüfung (`internal/server/router.go:147-177`), kein Limit pro Nutzer.
- Auswirkung: Ein authentifizierter Nutzer kann beliebig viele Streams öffnen (Goroutine + Kanal + offener Socket je Verbindung, RAM-Budget); hängende TCP-Verbindungen blockieren eine Goroutine in `Fprintf` für unbestimmte Zeit; Proxies/NAT trennen idle Streams ohne Heartbeat (Clients sehen das erst spät). Broadcast selbst blockiert nicht (non-blocking send mit `default`, `hub.go:32-38`) – Verifiziert OK. Cleanup: `defer` löscht Client und schließt Kanal (`hub.go:117-123`), `RevokeUser` schließt Streams (`:79-88`); kein Doppel-Close (Prüfung `if _, ok := h.clients[ch]`).
- Fix: Max. N Streams pro Nutzer (älteste schließen), Heartbeat alle 15–30 s (`: ping\n\n`) mit Write-Deadline je Write, Gesamtlimit. Aufwand: S.

### F-6-09 (S3) – CSV-Import verschluckt Fehler und ist nicht atomar
- Evidenz: `internal/core/export/service.go:173-181` fehlerhafte Zeilen per `continue`, `:232-236` Fehler von `contactSvc.Create` werden ignoriert (`if err == nil { count++ }`), Antwort nur `imported_count` (`handlers/export.go:59-60`). Keine Transaktion, keine Duplikaterkennung (`contacts.email` nicht unique, F-5-11). `maxRows = 5000` stoppt still.
- Auswirkung: Nutzer erfährt nicht, welche Zeilen scheitern; Teilimporte sind nicht rückgängig zu machen; erneuter Import erzeugt Duplikate. Der Import-Speicher ist dank Streaming-`csv.Reader` und 10-MB-Limit (`handlers/export.go:42`) begrenzt – Verifiziert OK.
- Fix: Fehlerliste (Zeile, Grund) zurückgeben, optional Dry-Run/Transaktion, Duplikate über E-Mail erkennen, Abbruchgrenze kommunizieren. Aufwand: M.

### F-6-10 (S3) – Uneinheitliches Fehlerformat, falscher Content-Type
- Evidenz: Zwei Muster nebeneinander: `writeJSONError(...)` (`handlers/ai.go:90-94`, ca. 120 Aufrufe) und `http.Error(w, `{"error":"..."}`, code)` in 15 Handler-Dateien (Zählung `grep -c 'http.Error(w, `{'`: emails 54, contacts 25, companies 21, deals 21, ai 19, notes 17, users 17, todos 15, ...). `http.Error` setzt `Content-Type: text/plain; charset=utf-8` (Go-Stdlib-Verhalten) trotz JSON-Body.
- Auswirkung: Clients, die `Content-Type` prüfen, können Fehler nicht einheitlich parsen; Fehlerschema variiert (`error` vs. `error`+`message`). Erhöht Risiko von F-6-06.
- Fix: Ein Helper im Paket `handlers` (z. B. `respondError`), `http.Error(w, '{` per Lint/Script verbieten. Aufwand: M (mechanisch).

### F-6-11 (S3) – Health/Readiness prüft nichts, Container ohne Server-/Worker-Healthcheck
- Evidenz: `internal/server/router.go:135-142` liefert konstant `healthy`, hartkodiert `"version":"3.0.0"`, kein DB-Ping, kein Migrationsstatus (vorhandener `db.GetMigrationStatus` wird nicht genutzt). `docker-compose.yml` enthält genau einen `healthcheck` (Zeile 84, Datenbank); Worker nutzt `condition: service_started`.
- Auswirkung: Orchestrierung sieht einen Server mit toter DB-Verbindung als gesund; Worker startet vor abgeschlossener Migration (F-5-05). Keine Liveness/Readiness-Trennung.
- Fix: `/api/v1/health/ready` mit `pool.Ping` + Migrationsstatus (ohne Interna), Compose-Healthchecks für Server, Worker `depends_on: service_healthy`. Version aus `BuildCommit`/ldflags. Aufwand: S.

### F-6-12 (S3) – Strukturelles Risiko `SELECT *` mit sensiblen Spalten
- Evidenz: 89 Vorkommen von `SELECT *`/`RETURNING *` in `internal/db/queries`; generierte Modelle tragen `json:"password_hash"`/`json:"totp_secret_encrypted"`/`json:"password_encrypted"` (`internal/db/models.go:260` für E-Mail-Konto, analog `User`). Aktuell mappen die Handler auf DTOs (`users.go:50-63`, `emails.go:141`) – aber Kontakte, Deals, Notizen etc. werden direkt als `db.*` serialisiert (z. B. `handlers/contacts.go:142-150`).
- Auswirkung: Künftige Spalten in diesen Tabellen (z. B. interne Felder) würden automatisch in API-Antworten erscheinen; ein vergessenes DTO bei `users`/`email_accounts` leakt Secrets. Aktuell kein konkreter Leak gefunden.
- Fix: Für `users`/`email_accounts` Spalten explizit selektieren (`-- name: ... :one` ohne `*`) oder `json:"-"` per sqlc-Override (`sqlc.yaml`: `emit_json_tags` + `overrides` für `users.password_hash`, `users.totp_secret_encrypted`, `email_accounts.password_encrypted`). Aufwand: S.

### F-6-13 (S3) – Verschluckte Fehler an kritischen Stellen
- Evidenz: `grep -rnE '_ = |_, _ =' internal cmd` -> 296 Treffer, davon relevant: `handlers/users.go:247,321` (`_ = h.audit.LogDiff` bei Rollen-/Statusänderung), `auth/service.go:139,149,201-264` (`_ = s.audit.Log` für Login-Fehlschläge – akzeptabel als Best Effort, aber ohne Metrik), `core/automation/service.go:110,287,575,611` (`_ = json.Unmarshal` bei Workflow-Schritten/Snapshots -> fehlerhafte JSON-Schritte werden zu leeren Workflows), `core/backup/restore.go:84` (Start-Audit), `core/email/service.go:313-316` (F-6-04), `core/export/service.go:175-181,232` (F-6-09), `launcher/*` (Dateioperationen mit `_ =`, z. B. `engine.go:568,602,647`, schreiben Backups ohne Fehlerprüfung).
- Positiv: `panic` nur bei Initialisierungsfehlern (`auth/service.go:44`, `auth/csrf.go:29`, `router.go:62`) und `http.ErrAbortHandler` (`handlers/backup.go:83`); Fehler-Wrapping mit `%w` üblich (121 Stellen in `core` vs. 31 `fmt.Errorf` ohne `%w`, meist Validierungsfehler).
- Fix: Audit-Fehler bei sicherheitsrelevanten Aktionen nicht ignorieren (F-5-06), `json.Unmarshal`-Fehler loggen/zurückgeben, Launcher-Backups Fehler melden. Aufwand: M.

### F-6-14 (S4) – Logging: unstrukturiert, PII/Token-Risiko, Request-ID ungenutzt
- Evidenz: 62 `log.Printf/Println` vs. 1 `slog`-Nutzung (Grep); `middleware.Logger` (`router.go:100`) loggt vollständige URL inkl. Query, darunter `GET /api/v1/auth/accept-invite?token=<Klartext>` (`handlers/invites.go:39-44`); Request-ID-Middleware aktiv (`router.go:98`), aber `log.Printf` der Handler enthält sie nicht. Login-Logs mit E-Mail `auth/service.go:283,295` (`user.Email`) und Audit `email` im Klartext (`:209,223`). Keine Prompts/Mailinhalte in Logs gefunden (Stichprobe `grep -iE 'token|password|body|prompt|secret|email'` auf `log.Printf` in server/core/auth/ai: nur IDs und E-Mail-Adressen).
- Auswirkung: Einmal-Einladungstoken landen in Access-Logs (einmalig verwendbar, aber bis Ablauf nutzbar); E-Mail-Adressen sind personenbezogene Daten im Log; Korrelation über Request-ID nicht möglich.
- Fix: Query-Strings im Logger maskieren (Token als POST-Body oder Fragment), `slog` mit Request-ID-Feld, Nutzer-ID statt E-Mail loggen. Aufwand: M.

### F-6-15 (S4) – Doppelpflege: In-Memory-Querier und In-Memory-Fallbacks in Core-Services
- Evidenz: `internal/db/demo/querier.go` (2898 Zeilen) spiegelt `db.Querier`; Core-Services besitzen zusätzlich eigene Speicherpfade mit Mutex (`core/note/service.go:260-295` `s.notes`, `core/appointment/service.go` `s.inMem`, Zeile 304).
- Auswirkung: Jede Query/Änderung muss dreifach gepflegt werden (SQL, Demo-Querier, Service-Fallback); Drift zwischen Demo und echter DB möglich (E2E läuft nur gegen Demo, vgl. F-5-01).
- Fix: Fallbacks entfernen, Demo nur über den Querier; Contract-Tests, die beide Implementierungen gegen dieselbe Spezifikation prüfen. Aufwand: L.

### F-6-16 (S4) – `golangci-lint` lokal nicht lauffähig
- Evidenz: `export PATH=$PATH:$HOME/go/bin; golangci-lint run ./...` endet mit Typecheck-Fehlern (`could not import sync/atomic ... export data version 4 is greater than maximum supported version 2`, `undefined: chi`, `undefined: imap`). Das Binary wurde mit einem älteren Go-Toolchain-Stand gebaut als Go 1.26 des Moduls.
- Auswirkung: Der Lint-Teil des Gates (`.golangci.yml`: gosec, errcheck, bodyclose) ist auf dieser Maschine nicht prüfbar; Findings wie F-6-13 wurden daher manuell gesucht. `make lint` nutzt nur `go vet` (AGENTS.md).
- Fix: `golangci-lint` mit Go 1.26 neu bauen (`go install .../golangci-lint@latest`), Version im CI pinnen. Aufwand: S.

## Step 4 – RAM-Budget (Zusammenfassung)

Kritische Stellen mit unbegrenztem Speicher: IMAP-Sync (F-6-03), Audit-Verifikation über `ListAuditLogsAsc` (F-5-09), unbegrenzte Listenqueries (F-5-09/F-6-05), SSE ohne Client-Limit (F-6-08). Unkritisch: CSV-Import (Streaming, 10 MB), CSV-Export (`Limit: 10000`, `bytes.Buffer`, `core/export/service.go:41-50`), Backup (Streaming `row_to_json` + Spool-Datei, `core/backup/restore.go:47-55`), Request-Bodies (globales Limit 5/50/100 MB, `router.go:118-131`, plus `MaxBytesReader` in Handlern). Hinweis: Das globale 100-MB-Limit für Restore (`router.go:123`) wird bewusst auf Platte gespoolt, nicht im RAM gehalten (`handlers/backup.go:153`). Ungemessen: tatsächlicher Verbrauch unter `make simulate`.

## Verifiziert OK (mit Evidenz)

- Body-Limits: globale Middleware für POST/PUT/PATCH mit 5 MB Standard, 50 MB Storage/CSV, 100 MB Restore (`internal/server/router.go:118-131`); Upload-Größe zusätzlich im Storage begrenzt (`internal/storage/local.go:74`, `io.LimitReader`); KI-Recherche liest nur 256 KB (`internal/ai/research.go:253`).
- HTTP-Server mit Timeouts (`cmd/server/main.go:233-238` Read 15 s, Write 30 s) und Graceful Shutdown mit 5 s (`:247-252`); SSE hebt das Write-Deadline gezielt pro Stream auf.
- SSE-Broadcast blockiert nie: nicht blockierendes Senden mit Puffer 16 (`sse/hub.go:32-38,69`), Cleanup per `defer`, `RevokeUser` bei Rollen-/Statuswechsel, Session-Revoker eingebunden (`router.go:256-258`).
- Worker: Kontext-basiertes Shutdown mit `signal.NotifyContext` (`cmd/worker/main.go:41`), Retry der Client-Initialisierung (`:70-90`), Jobs mit Eingabevalidierung und `%w`-Wrapping (`queue/jobs.go:122-138,150-167`), Triage läuft über den Gateway mit Guard (Kommentar `jobs.go:141-143`; Guard selbst gehört zu Task 4).
- Externe HTTP-Clients mit Timeout: `core/automation/service.go:528` (5 s), `ai/gateway.go:142-148`, `ai/research.go:123-130`.
- Goroutines im Router enden mit dem Kontext (Limiter-Stop bei `cfg.Context.Done()`, `router.go:248-254,296-322`).
- Rows/Bodies: `defer rows.Close()` in Migrator (`db/migrator.go:72`); Restore und Scratch mit garantiertem Rollback (`core/backup/restore.go:99-106`).
- CSV-Export gegen Formel-Injektion geschützt (`sanitizeCSVField`, `core/export/service.go:29-36`, angewendet `:97-103`).
- Panics nur beim Start/Abbruch (siehe F-6-13), kein `panic` im Request-Pfad außer `http.ErrAbortHandler`.
- `Recoverer` und `RequestID` aktiv (`router.go:98,101`); `RealIP` bewusst entfernt (Spoofing, `router.go:99`).
- Kein Query-Parameter steuert `ORDER BY`/Spaltennamen: alle Sortierungen fest in SQL (Sortier-Whitelist damit nicht nötig); keine dynamisch zusammengesetzten SQL-Strings in `internal/core` (Grep `Sprintf` in `core/contact` zeigt nur Wertformatierung `service.go:126`).

## Nicht geprüft
- RAM-/Latenzmessung unter Last, echte IMAP/SMTP-Server, Race-Detector-Läufe (Controller führt Tests aus), Frontend (Task 7), PII-Guard-Abdeckung und Ownership in Handlern (Task 4).
