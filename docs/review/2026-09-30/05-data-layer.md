# Review 05 – Datenschicht & Migrationen

Stand: 2026-09-30, Commit `c6bac3e`. Read-only-Review (Task 5 des Review-Plans). Kein Produktcode geändert.
Severity: S1 kritisch, S2 hoch, S3 mittel, S4 niedrig. Aufwand: S/M/L.

Einschränkung: Es steht keine PostgreSQL-Instanz zur Verfügung (kein `postgres`/`psql`, Docker laut Auftrag nicht erlaubt). `EXPLAIN` an Kernqueries wurde daher **nicht** ausgeführt; Index-Aussagen beruhen auf statischer Analyse der Migrationen und Queries. F-5-01 ist statisch belegt (siehe dort), aber nicht gegen eine leere Datenbank ausgeführt.

## Step 1 – Invarianten

| Prüfung | Ergebnis | Beleg |
| --- | --- | --- |
| `diff -r migrations internal/db/migrations` | identisch (Symlink) | `migrations -> internal/db/migrations` (`ls -l`), `diff` ohne Ausgabe |
| Nummerierung | lückenlos 00001–00021, 5-stellig | `ls internal/db/migrations` |
| `sqlc generate` | keine Änderung an `internal/db` | `git diff --stat internal/db` leer, `git status --short` leer; kein Revert nötig |
| Down-Abschnitte | Migrator schneidet bei `-- +goose Down` ab (`migrator.go:141-143`). 00005–00009 haben keinen Marker, enthalten aber auch kein `DROP`/destruktives SQL (grep) | `internal/db/migrator.go:141`, grep über 00005–00009 |

## Findings

### F-5-01 (S1) – Frische Installation scheitert an Migration 00010 (Index auf nicht existierende Spalte)
- Evidenz: `internal/db/migrations/00010_foreign_key_indexes.sql:17` `CREATE INDEX ... ON notes (contact_id) WHERE contact_id IS NOT NULL`. Die Tabelle `notes` (`00008_notes_schema.sql:2-11`) hat nur `entity_type/entity_id`, und keine Migration fügt `notes.contact_id` hinzu (grep über alle Migrationen; `internal/db/models.go:337-347` `Note` ohne `ContactID`). Das Repo bestätigt es selbst: `scripts/test-backup-integration.sh:28-32` ("indexes notes(contact_id), a column that no migration ever creates, so replaying the historical migration set from an empty database cannot work") und überspringt 00010 (`SKIP_MIGRATION`, Zeile 48).
- Auswirkung: `db.RunMigrations` (`cmd/server/main.go:214`) läuft in der Transaktion in einen SQL-Fehler und `log.Fatalf` beendet den Server. Jede Neuinstallation auf leerer DB (Kern-Deployment-Szenario) startet nicht. Bestehende Installationen sind nur betroffen, wenn ihr Schema aus dem Legacy-Stand stammt (dort existiert die Spalte offenbar). E2E-Tests laufen im Demo-Modus mit In-Memory-Querier (`main.go:207-208`) und fangen das nicht ab.
- Nicht ausgeführt: gegen echte DB nicht reproduziert. Statisch eindeutig, plus Selbstbeleg des Repos.
- Fix: 00010 so entschärfen, dass Zeile 17 entfällt (oder per `DO $$ ... IF EXISTS (information_schema.columns ...)` geschützt wird). Da der Migrator keine Checksummen prüft (F-5-05), ist die Änderung für bereits angewendete Installationen folgenlos; das widerspricht formal der AGENTS-Regel "nie angewendete Migration editieren", daher bewusste Entscheidung dokumentieren. Danach Skip-Hack im Integrationsskript entfernen und einen CI-Lauf "leere DB -> alle Migrationen" ergänzen (Testcontainer/CI-Postgres). Aufwand: S (Fix), M (CI-Gate).

### F-5-02 (S2) – Backup/Restore verliert die Audit-Hash-Kette
- Evidenz: `internal/db/queries/backup.sql:278-296` `UpsertAuditLogsRow` schreibt `prev_hash`/`hash` weder ein noch aus dem JSON zurück (Spaltenliste: id, user_id, entity_type, entity_id, action, changes, ip_address, user_agent, created_at, old_values, new_values). Ebenso `UpsertAIAuditLogsRow` (`backup.sql:572-575`) ohne die in 00021 ergänzte Spalte `user_id`.
- Auswirkung: Nach einem Restore (Replace-Modus) haben alle Audit-Zeilen `prev_hash = NULL`; `VerifyAuditChain` behandelt sie als "Legacy" und überspringt sie (`internal/core/audit/service.go:275-282`). Die kryptografische Integritätsgarantie (SP8c) ist nach jedem Restore stillschweigend weg, ohne Fehlermeldung. AI-Audit-Attribution (`user_id`) geht verloren.
- Fix: Spalten `prev_hash, hash` (und `ai_audit_logs.user_id`) in Upsert-Statement, Spalten-Allowlist (`internal/core/backup/tables.go` ~328ff) und Tests aufnehmen; sqlc regenerieren. Aufwand: S.

### F-5-03 (S2) – Hash-Kette umgehbar: Zeilen mit `prev_hash IS NULL` werden nicht verifiziert
- Evidenz: `internal/core/audit/service.go:275-282`: `if !l.PrevHash.Valid || l.PrevHash.String == "" { ... continue }`. Ein Angreifer mit DB-Schreibzugriff (oder Admin über Restore-Merge, `UpsertAuditLogsRow` macht `ON CONFLICT DO UPDATE`, `backup.sql:286`) setzt `prev_hash=NULL` und ändert die Zeile beliebig; `VerifyAuditChain` meldet weiterhin "valid".
- Auswirkung: Manipulationserkennung nur so stark wie ihre schwächste Regel. Zusätzlich: es gibt keinen DB-seitigen Schutz gegen `UPDATE`/`DELETE` auf `audit_logs` (kein Trigger/REVOKE; `grep TRIGGER|REVOKE internal/db/migrations` leer). Das Löschen der letzten Zeilen (Truncation) ist ebenfalls nicht erkennbar.
- Fix: Legacy-Toleranz nur für Zeilen vor dem Zeitstempel der Migration 00021 (Grenze, z. B. erste Zeile mit Hash); danach `prev_hash` NOT NULL erzwingen. Trigger `BEFORE UPDATE OR DELETE ON audit_logs` mit Exception (Restore/Truncate über dedizierten Pfad). Optional Kettenanker extern speichern. Aufwand: M.

### F-5-04 (S2) – Race beim Anhängen an die Hash-Kette
- Evidenz: `internal/core/audit/service.go:178-188` (`getLatestHash`, separater SELECT) und `:213-215` / `:243-245` (Hash berechnen, dann `CreateAuditLog`). Kein Mutex, keine Transaktion, kein `pg_advisory_xact_lock` (grep `Mutex|pg_advisory` in `core/audit` leer). `GetLatestAuditLogHash` sortiert nach `created_at DESC, id DESC` (`queries/audit.sql:20-24`).
- Auswirkung: Zwei parallele Requests (oder Server + Worker, `cmd/worker/main.go:47` erzeugt einen eigenen `audit.Service`) lesen denselben `prev_hash`; die Kette verzweigt. `VerifyAuditChain` meldet dann "broken" (`service.go:285`) obwohl nichts manipuliert wurde -> Fehlalarm bzw. Kette dauerhaft unverifizierbar. Auch `created_at`-Gleichstand mit zufälliger UUID-Reihenfolge kann die Reihenfolge verfälschen.
- Fix: Anhängen in einer Transaktion mit `pg_advisory_xact_lock(<konstante>)`, Latest-Hash innerhalb der Tx lesen (oder Sequenzspalte `BIGSERIAL` als Ordnungskriterium). Aufwand: M.

### F-5-05 (S3) – Migrator: kein Lock, keine Checksummen, Worker nicht an Migrationsstand gekoppelt
- Evidenz: `internal/db/migrator.go:108-172`: prüft `schema_migrations` pro Datei ohne `pg_advisory_lock`; keine Checksumme der Datei (Tabelle hat nur `version`, `applied_at`, `migrator.go:110-115`). `GetMigrationStatus` ignoriert Query-Fehler (`migrator.go:71-83`, `if err == nil`). `docker-compose.yml`: Worker hängt nur mit `condition: service_started` am Server (Grep `depends_on`), nicht an `service_healthy`; nur der Server ruft `RunMigrations` (`cmd/server/main.go:214`; `cmd/worker/main.go` enthält keinen Migrationsaufruf).
- Auswirkung: (a) Zwei gleichzeitig startende Instanzen laufen in PK-Konflikt bzw. doppelte Ausführung, endet in `Fatalf` (Tx-Rollback schützt Daten). (b) Nachträgliche Änderung einer angewendeten Migration bleibt unbemerkt (verstärkt F-5-01). (c) Beim Upgrade kann der Worker vor Abschluss der Migration mit altem Schema arbeiten -> Jobfehler/Retries.
- Fix: `SELECT pg_advisory_lock(hash)` um die Schleife, Spalte `checksum` (SHA-256) mit Warn-/Fehlerlog bei Abweichung, Worker mit Healthcheck-Abhängigkeit (`/api/v1/health` mit DB-Check, siehe F-6-11) oder eigener `GetMigrationStatus`-Prüfung beim Start. Aufwand: M.

### F-5-06 (S3) – Mehrfach-Schreibvorgänge und Audit nicht transaktional
- Evidenz: `db.RunTx` (`internal/db/tx.go:10`) wird nur in `internal/auth/service.go:87,441` genutzt (grep `RunTx` außerhalb Tests/Demo). Deal-Update schreibt zuerst, danach Audit; Audit-Fehler wird nur geloggt: `internal/core/deal/service.go:187-191` (`log.Printf("[audit] failed ...")`). Ähnlich `_ = h.audit.LogDiff(...)` bei Rollen-/Statusänderung `internal/server/handlers/users.go:247,321`.
- Auswirkung: Erfolgreiche Änderung ohne Audit-Eintrag möglich (DB-Hiccup). Für ein DSGVO-/Audit-Versprechen (Rollenänderung!) unzureichend; Kette bleibt konsistent, aber lückenhaft.
- Fix: Für sicherheitsrelevante Aktionen (Rolle, Status, Export, Backup, Ownership) Audit in derselben Tx via `RunTx` und Fehler als Abbruch behandeln; bei Fachdaten mindestens Fehlerzähler/Metrik. Aufwand: M.

### F-5-07 (S3) – Hard Delete kaskadiert und polymorphe Referenzen ohne FK
- Evidenz: `todos.contact_id/deal_id ... ON DELETE CASCADE` (`00002_core_crm_schema.sql:80-81`), `call_activities.contact_id ON DELETE CASCADE` (`00006:19`), `company_research.company_id CASCADE` (`00005:19`). `notes.entity_id` (`00008:4`), `workflow_runs.target_id TEXT` (`00007:19`), `audit_logs.entity_id UUID NOT NULL` ohne FK. `DeleteContact` ist echtes `DELETE` (`queries/contacts.sql`, `DeleteContact`).
- Auswirkung: Löschen eines Kontakts löscht Todos und Anrufe still mit, lässt aber Notizen, Workflow-Runs und Termin-Verknüpfungen (SET NULL) als Waisen bzw. entkoppelt zurück. Kein Soft-Delete: keine Wiederherstellung außer Backup. Für Notizen mit personenbezogenem Inhalt kann das aus DSGVO-Sicht (Löschkonzept) relevant sein: nach "Kontakt löschen" bleiben Notizen erhalten.
- Positiv: Audit-Tabelle hat keine kaskadierenden FKs auf Fachtabellen (nur `user_id ... SET NULL`), Löschen eines Kontakts löscht also kein Audit.
- Fix: Löschkonzept festlegen (Notizen/Runs mit löschen oder anonymisieren) und in derselben Tx umsetzen; optional Soft-Delete für Kontakte/Deals. Aufwand: M.

### F-5-08 (S3) – Nutzerlöschung würde Audit-Hash brechen (latent)
- Evidenz: `audit_logs.user_id ... ON DELETE SET NULL` (`00002:89`), aber `user_id` ist Teil des Hash (`internal/core/audit/service.go:152-160`, `userStr`). Aktuell gibt es keinen `DELETE FROM users`-Pfad im Code (grep `DELETE FROM users|DeleteUser` in queries/handlers/core leer); Nutzer werden deaktiviert.
- Auswirkung: Ein künftiges "Benutzer löschen"/DSGVO-Löschen ändert Audit-Zeilen und macht die Kette ungültig.
- Fix: Nutzer-ID in Audit nie nullen (FK entfernen oder `ON DELETE NO ACTION`) und stattdessen pseudonymisieren; oder `user_id` aus dem Hash nehmen und Pseudonym-Tabelle nutzen. Aufwand: M.

### F-5-09 (S3) – Unbegrenzte bzw. im Speicher paginierte Queries
- Evidenz (awk-Auswertung aller `:many`-Queries ohne `LIMIT`): `ListNotes`, `ListNotesByEntity`, `ListAppointments`, `ListDealsByStage`, `ListWorkflowRuns`, `ListKBArticles`, `ListAIResearchJobs`, `ListEmailTemplates`, `ListUsers`, `ListTodosByAssignee`, `ListContactsByCompany`, `ListEmailMessagesByThread`, `ListUnownedContacts/Companies`, `ListAuditLogsAsc`. Dazu wird in `internal/core/deal/service.go:196-227` (`ListByStage`) und `internal/core/appointment/service.go:113-127` erst alles geladen und dann im Speicher geschnitten ("Pagination" nur kosmetisch). `ListAuditLogsAsc` lädt die gesamte Audit-Tabelle für `VerifyAuditChain` (`audit/service.go:266`).
- Auswirkung: Speicher- und Latenzwachstum mit Datenmenge (RAM-Budget < 420 MB); `VerifyAuditChain` ist bei einigen 100k Zeilen ein Speicher-Spike. Siehe auch F-6-05.
- Fix: `LIMIT/OFFSET` in SQL (Keyset-Pagination bevorzugt), Verifikation streamend/chunkweise mit `pgx.Rows`. Aufwand: M.

### F-5-10 (S3) – Suche und Sortierung nicht durch Indizes gedeckt
- Evidenz: `SearchContacts` filtert `first_name ILIKE '%..%' OR last_name ILIKE ... OR email ILIKE ... OR phone ILIKE ...` (`queries/contacts.sql`, SearchContacts). Vorhandener Trigram-Index ist auf den Ausdruck `(first_name || ' ' || last_name)` (`00002:101`) und passt zu keinem dieser Prädikate; für `email` gibt es nur B-Tree (`00002:100`), `phone` gar keinen. `ListContacts` sortiert nach `last_name, first_name` ohne passenden Index. `ListDeals`/`ListTodos`-Sortierung nach `created_at` bzw. `due_date` teils ohne Index (`idx_todos_status_due` nur bei Statusfilter). Nutzereingabe wird ungefiltert in `%…%` gesetzt (LIKE-Metazeichen `%`/`_` nicht escaped).
- Auswirkung: Sequenzscans über `contacts` bei jeder Suche; bei Zehntausenden Kontakten spürbar. Nicht per `EXPLAIN` gemessen (keine DB).
- Fix: GIN-Trigram-Indizes auf `first_name`, `last_name`, `email`, `phone` (oder ein gemeinsamer `search_text`), LIKE-Escape; Index `(owner_user_id, last_name, first_name)`. Aufwand: S–M.

### F-5-11 (S4) – Redundante Indizes, fehlende Constraints
- Duplikate: `idx_audit_entity` (00002:106) = `idx_audit_logs_entity` (00010:33); `idx_ai_audit_logs_created_at` (00005:28) = `idx_ai_audit_logs_created` (00010:36); `idx_email_messages_account` (00003:62) = `idx_email_messages_account_id` (00010:25); `idx_email_messages_thread` = `idx_email_messages_thread_id`; `idx_appointments_contact_id` doppelt (00009:14 und 00010:20, ein Name, Teilindex wird via IF NOT EXISTS übersprungen); `idx_deals_assigned` (00002:104) = `idx_deals_assigned_to` (00010:11, partial); `idx_users_email` (00001:38) redundant zu `UNIQUE(email)`; `idx_intake_forms_slug` (00006:37) redundant zu `UNIQUE`.
- Constraints: `deals.stage VARCHAR(50)` ohne CHECK/Enum, `deals.probability` ohne `CHECK (0..100)`, `deals.value` ohne `CHECK (>= 0)`; `contacts.email` ohne UNIQUE (Duplikate per CSV-Import); `email_messages.message_id UNIQUE` global statt `(account_id, message_id)` (siehe F-6-04).
- Auswirkung: Schreib-Overhead und Speicher (gering); Datenqualität nur in Anwendungscode gesichert.
- Fix: Aufräum-Migration (neue Nummer, nicht 00010 ändern), CHECK-Constraints ergänzen. Aufwand: S.

### F-5-12 (S4) – Migrationshygiene
- 00005–00009 ohne `-- +goose Up/Down`-Marker (uneinheitlich; funktional harmlos, s. Step 1). 00011 `ALTER TYPE ... ADD VALUE` in Transaktion (ab PG12 erlaubt, Wert erst nach Commit nutzbar; hier kein Gebrauch in derselben Migration) und ohne Down (dokumentiert, `00011:6`). 00020 `UPDATE`-Backfill auf `notes`/`appointments` ohne Batch (Tabellen klein, Lock-Risiko gering). `migrator.go:150` führt die gesamte Datei als ein `tx.Exec`, was Multi-Statement per Simple-Protocol erlaubt, aber keine `CREATE INDEX CONCURRENTLY` zulässt (keine vorhanden).
- Fix: Marker vereinheitlichen, Lint-Regel im Hook. Aufwand: S.

## Verifiziert OK (mit Evidenz)

- Migrations-Symlink intakt, keine Abweichung (`diff -r`, `ls -l`).
- `sqlc generate` erzeugt keinen Drift: generierter Code aktuell (`git diff --stat internal/db` leer).
- Passwort-Hash/TOTP-Secret erreichen keine API-Antworten: `UserResponse` mappt explizit nur unkritische Felder (`internal/server/handlers/users.go:50-63`), E-Mail-Konto-Antwort nutzt `HasPassword` statt `password_encrypted` (`server/handlers/emails.go:141`), obwohl die Queries `SELECT *` verwenden (89 Vorkommen von `SELECT *`/`RETURNING *`; Risiko bleibt strukturell, siehe F-6-12).
- Wissensdatenbank-Queries lesen den Embedding-Vektor nie in Listen (`queries/knowledge_base.sql:19-24` benennt Spalten und nutzt `(embedding IS NOT NULL) AS indexed`).
- pgvector: `vector(1024)` und HNSW-Index `vector_cosine_ops` (`00015:10-15`), Suche `ORDER BY embedding <=> $1 LIMIT n` nutzt den Index (`knowledge_base.sql:31-35`); Dimension konsistent mit Backup-Kontrakt (`backup.sql` Kopfkommentar). Migration 00015 entwertet alte Vektoren bewusst (`UPDATE ... SET embedding = NULL`) und dokumentiert Re-Embedding.
- Refresh-Token-Rotation atomar: `ConsumeRefreshToken` ist ein einzelnes `UPDATE ... WHERE used_at IS NULL ... RETURNING` (`queries/sessions.sql:22-29`), Replay-Erkennung über `GetRefreshTokenByHash` und `DeleteRefreshTokenFamily`.
- Einladungs-Annahme race-sicher: CTE mit `FOR UPDATE`, Consume und Aktivierung in einem Statement; Einladung und Nutzer werden in einem Statement angelegt (`queries/invites.sql:15-49`). Token nur als SHA-256-Hash gespeichert (`user_invites.token_hash`, `00018:6`).
- Eigentümer-Filter in Listen-/Suchqueries vorhanden: `ListContacts`, `SearchContacts`, `ListDealsByStage`, `ListTodos`, `ListEmailAccounts` (`sqlc.narg('owner_user_id')`/`assigned_to`); Einzelabfragen (`GetContactByID`, `GetTodoByID`) filtern nicht auf Ownership, das ist Sache der Handler (Task 4).
- Fremdschlüssel-Indizes für die meisten FK-Spalten vorhanden (00010, 00020: `idx_*_owner_user_id`, `idx_refresh_tokens_family`).
- Restore-Design: eine Tx für Truncate+Upserts, Vorabprüfung auf vollständigen Tabellenbestand im Replace-Modus, Rollback mit `context.WithoutCancel` (`internal/core/backup/restore.go:47-131`); Statements parametrisiert über `jsonb_to_record`, keine dynamischen Identifier.
- Alle `CREATE TABLE`-Migrationen ab 00018 laufen in einer Transaktion je Datei (`migrator.go:149-171`), Fehler führen zu Rollback ohne Eintrag in `schema_migrations`.
- Enum-Mapping via `sqlc.yaml` auf `string` wie in AGENTS.md beschrieben (`sqlc.yaml` overrides).

## Nicht geprüft
- `EXPLAIN`-Pläne, Lock-Verhalten großer Tabellen unter Last, tatsächliches Verhalten von 00010 auf leerer DB (keine Postgres-Instanz).
- Ownership-Prüfung in den Handlern (Task 4).
