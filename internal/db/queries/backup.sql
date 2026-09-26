-- internal/db/queries/backup.sql
--
-- B1/B2: canonical SQL for the bounded-memory backup export, the static
-- restore upserts, and the reverse-ordered replace-mode truncation.
--
-- Contract rules that this file is the single source of truth for:
--
--   * Every selector returns `row_to_json(t)` ordered by primary key, so the
--     wire format keeps JSONB/array/vector columns as JSON values instead of
--     base64 strings.
--   * Every restore statement is `:exec` and takes exactly one bind parameter,
--     `sqlc.arg(row)::jsonb`. No identifier is ever assembled from request
--     data.
--   * Restore uses typed `jsonb_to_record` fields, never `r->>'field'`: a JSON
--     null must yield SQL NULL, which `''::uuid` would turn into a cast error.
--   * TEXT[] columns are rebuilt with
--     `ARRAY(SELECT jsonb_array_elements_text(COALESCE(r.col, '[]'::jsonb)))`
--     and the embedding is cast back to `vector(1024)`. The 1024-dimensional
--     embedding contract is never reduced to the obsolete 768 contract.
--   * `workflows`, `workflow_runs` and `workflow_steps` keep text primary and
--     foreign keys, so `r.id`, `r.workflow_id` and `r.run_id` stay uncast.
--   * Enum and vector types are schema-qualified with `public.`. The scratch
--     restore drill pins `search_path` to the scratch schema only, so an
--     unqualified `user_role` or `vector` would be unresolvable there. Only the
--     *types* are qualified; table names stay unqualified on purpose, so a
--     missing scratch table fails instead of silently resolving to public.

-- name: ExportUsersRows :many
SELECT row_to_json(t) FROM users t ORDER BY t.id;

-- name: UpsertUsersRow :exec
INSERT INTO users (id, email, password_hash, first_name, last_name, role, status,
                   totp_secret_encrypted, totp_enabled, last_login_at, created_at, updated_at, totp_last_used_step)
SELECT r.id::uuid, r.email, r.password_hash, r.first_name, r.last_name,
       r.role::public.user_role, r.status::public.user_status,
       r.totp_secret_encrypted, r.totp_enabled::boolean, r.last_login_at::timestamptz,
       r.created_at::timestamptz, r.updated_at::timestamptz, r.totp_last_used_step::bigint
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, email text, password_hash text, first_name text, last_name text,
    role text, status text, totp_secret_encrypted text, totp_enabled text,
    last_login_at text, created_at text, updated_at text, totp_last_used_step text)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    password_hash = EXCLUDED.password_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    role = EXCLUDED.role,
    status = EXCLUDED.status,
    totp_secret_encrypted = EXCLUDED.totp_secret_encrypted,
    totp_enabled = EXCLUDED.totp_enabled,
    last_login_at = EXCLUDED.last_login_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    totp_last_used_step = EXCLUDED.totp_last_used_step;

-- name: ExportCompaniesRows :many
SELECT row_to_json(t) FROM companies t ORDER BY t.id;

-- name: UpsertCompaniesRow :exec
INSERT INTO companies (id, name, domain, phone, email, address_street, address_zip,
                       address_city, address_country, custom_fields, created_at, updated_at)
SELECT r.id::uuid, r.name, r.domain, r.phone, r.email, r.address_street, r.address_zip,
       r.address_city, r.address_country, r.custom_fields,
       r.created_at::timestamptz, r.updated_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, name text, domain text, phone text, email text, address_street text,
    address_zip text, address_city text, address_country text, custom_fields jsonb,
    created_at text, updated_at text)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    domain = EXCLUDED.domain,
    phone = EXCLUDED.phone,
    email = EXCLUDED.email,
    address_street = EXCLUDED.address_street,
    address_zip = EXCLUDED.address_zip,
    address_city = EXCLUDED.address_city,
    address_country = EXCLUDED.address_country,
    custom_fields = EXCLUDED.custom_fields,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ExportWorkflowsRows :many
SELECT row_to_json(t) FROM workflows t ORDER BY t.id;

-- name: UpsertWorkflowsRow :exec
INSERT INTO workflows (id, name, description, trigger_type, target_type, is_active,
                       steps_json, created_at, updated_at)
SELECT r.id, r.name, r.description, r.trigger_type, r.target_type, r.is_active::boolean,
       r.steps_json, r.created_at::timestamptz, r.updated_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, name text, description text, trigger_type text, target_type text,
    is_active text, steps_json jsonb, created_at text, updated_at text)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    trigger_type = EXCLUDED.trigger_type,
    target_type = EXCLUDED.target_type,
    is_active = EXCLUDED.is_active,
    steps_json = EXCLUDED.steps_json,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ExportContactsRows :many
SELECT row_to_json(t) FROM contacts t ORDER BY t.id;

-- name: UpsertContactsRow :exec
INSERT INTO contacts (id, company_id, first_name, last_name, email, phone, mobile, position,
                      lead_source, address_street, address_zip, address_city, latitude, longitude,
                      custom_fields, created_at, updated_at, consent_phone, consent_email,
                      consent_updated_at, stromverbrauch_kwh, gasverbrauch_kwh, zaehlernummer,
                      eigentuemer_status, tags)
SELECT r.id::uuid, r.company_id::uuid, r.first_name, r.last_name, r.email, r.phone, r.mobile,
       r.position, r.lead_source, r.address_street, r.address_zip, r.address_city,
       r.latitude::double precision, r.longitude::double precision, r.custom_fields,
       r.created_at::timestamptz, r.updated_at::timestamptz,
       r.consent_phone::boolean, r.consent_email::boolean, r.consent_updated_at::timestamptz,
       r.stromverbrauch_kwh::numeric, r.gasverbrauch_kwh::numeric, r.zaehlernummer, r.eigentuemer_status,
       r.tags
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, company_id text, first_name text, last_name text, email text, phone text,
    mobile text, position text, lead_source text, address_street text, address_zip text,
    address_city text, latitude text, longitude text, custom_fields jsonb, created_at text,
    updated_at text, consent_phone text, consent_email text, consent_updated_at text,
    stromverbrauch_kwh text, gasverbrauch_kwh text, zaehlernummer text, eigentuemer_status text,
    tags jsonb)
ON CONFLICT (id) DO UPDATE SET
    company_id = EXCLUDED.company_id,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    email = EXCLUDED.email,
    phone = EXCLUDED.phone,
    mobile = EXCLUDED.mobile,
    position = EXCLUDED.position,
    lead_source = EXCLUDED.lead_source,
    address_street = EXCLUDED.address_street,
    address_zip = EXCLUDED.address_zip,
    address_city = EXCLUDED.address_city,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    custom_fields = EXCLUDED.custom_fields,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    consent_phone = EXCLUDED.consent_phone,
    consent_email = EXCLUDED.consent_email,
    consent_updated_at = EXCLUDED.consent_updated_at,
    stromverbrauch_kwh = EXCLUDED.stromverbrauch_kwh,
    gasverbrauch_kwh = EXCLUDED.gasverbrauch_kwh,
    zaehlernummer = EXCLUDED.zaehlernummer,
    eigentuemer_status = EXCLUDED.eigentuemer_status,
    tags = EXCLUDED.tags;

-- name: ExportDealsRows :many
SELECT row_to_json(t) FROM deals t ORDER BY t.id;

-- name: UpsertDealsRow :exec
INSERT INTO deals (id, title, company_id, contact_id, value, currency, stage, probability,
                   assigned_to, closed_at, custom_fields, created_at, updated_at,
                   widerrufen_at, widerruf_grund, tags)
SELECT r.id::uuid, r.title, r.company_id::uuid, r.contact_id::uuid, r.value::numeric, r.currency,
       r.stage, r.probability::integer, r.assigned_to::uuid, r.closed_at::timestamptz,
       r.custom_fields, r.created_at::timestamptz, r.updated_at::timestamptz,
       r.widerrufen_at::timestamptz, r.widerruf_grund, r.tags
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, title text, company_id text, contact_id text, value text, currency text,
    stage text, probability text, assigned_to text, closed_at text, custom_fields jsonb,
    created_at text, updated_at text, widerrufen_at text, widerruf_grund text, tags jsonb)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    company_id = EXCLUDED.company_id,
    contact_id = EXCLUDED.contact_id,
    value = EXCLUDED.value,
    currency = EXCLUDED.currency,
    stage = EXCLUDED.stage,
    probability = EXCLUDED.probability,
    assigned_to = EXCLUDED.assigned_to,
    closed_at = EXCLUDED.closed_at,
    custom_fields = EXCLUDED.custom_fields,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    widerrufen_at = EXCLUDED.widerrufen_at,
    widerruf_grund = EXCLUDED.widerruf_grund,
    tags = EXCLUDED.tags;

-- name: ExportEmailAccountsRows :many
SELECT row_to_json(t) FROM email_accounts t ORDER BY t.id;

-- name: UpsertEmailAccountsRow :exec
INSERT INTO email_accounts (id, name, email_address, provider, imap_host, imap_port, smtp_host,
                            smtp_port, username, password_encrypted, is_active, last_synced_at,
                            created_at, updated_at, account_type, owner_user_id, last_sync_at, last_uid)
SELECT r.id::uuid, r.name, r.email_address, r.provider, r.imap_host, r.imap_port::integer,
       r.smtp_host, r.smtp_port::integer, r.username, r.password_encrypted, r.is_active::boolean,
       r.last_synced_at::timestamptz, r.created_at::timestamptz, r.updated_at::timestamptz,
       r.account_type, r.owner_user_id::uuid, r.last_sync_at::timestamptz, r.last_uid::bigint
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, name text, email_address text, provider text, imap_host text, imap_port text,
    smtp_host text, smtp_port text, username text, password_encrypted text, is_active text,
    last_synced_at text, created_at text, updated_at text, account_type text, owner_user_id text,
    last_sync_at text, last_uid text)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    email_address = EXCLUDED.email_address,
    provider = EXCLUDED.provider,
    imap_host = EXCLUDED.imap_host,
    imap_port = EXCLUDED.imap_port,
    smtp_host = EXCLUDED.smtp_host,
    smtp_port = EXCLUDED.smtp_port,
    username = EXCLUDED.username,
    password_encrypted = EXCLUDED.password_encrypted,
    is_active = EXCLUDED.is_active,
    last_synced_at = EXCLUDED.last_synced_at,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    account_type = EXCLUDED.account_type,
    owner_user_id = EXCLUDED.owner_user_id,
    last_sync_at = EXCLUDED.last_sync_at,
    last_uid = EXCLUDED.last_uid;

-- name: ExportTodosRows :many
SELECT row_to_json(t) FROM todos t ORDER BY t.id;

-- name: UpsertTodosRow :exec
INSERT INTO todos (id, title, description, due_date, status, priority, assigned_to, contact_id,
                   deal_id, created_at, updated_at)
SELECT r.id::uuid, r.title, r.description, r.due_date::timestamptz,
       r.status::public.todo_status, r.priority::public.todo_priority,
       r.assigned_to::uuid, r.contact_id::uuid, r.deal_id::uuid,
       r.created_at::timestamptz, r.updated_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, title text, description text, due_date text, status text, priority text,
    assigned_to text, contact_id text, deal_id text, created_at text, updated_at text)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    due_date = EXCLUDED.due_date,
    status = EXCLUDED.status,
    priority = EXCLUDED.priority,
    assigned_to = EXCLUDED.assigned_to,
    contact_id = EXCLUDED.contact_id,
    deal_id = EXCLUDED.deal_id,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ExportRefreshTokensRows :many
SELECT row_to_json(t) FROM refresh_tokens t ORDER BY t.id;

-- name: UpsertRefreshTokensRow :exec
INSERT INTO refresh_tokens (id, user_id, token_hash, user_agent, ip_address, expires_at, created_at)
SELECT r.id::uuid, r.user_id::uuid, r.token_hash, r.user_agent, r.ip_address,
       r.expires_at::timestamptz, r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, user_id text, token_hash text, user_agent text, ip_address text,
    expires_at text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    token_hash = EXCLUDED.token_hash,
    user_agent = EXCLUDED.user_agent,
    ip_address = EXCLUDED.ip_address,
    expires_at = EXCLUDED.expires_at,
    created_at = EXCLUDED.created_at;

-- name: ExportAuditLogsRows :many
SELECT row_to_json(t) FROM audit_logs t ORDER BY t.id;

-- name: UpsertAuditLogsRow :exec
INSERT INTO audit_logs (id, user_id, entity_type, entity_id, action, changes, ip_address,
                        user_agent, created_at, old_values, new_values)
SELECT r.id::uuid, r.user_id::uuid, r.entity_type, r.entity_id::uuid, r.action, r.changes,
       r.ip_address, r.user_agent, r.created_at::timestamptz, r.old_values, r.new_values
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, user_id text, entity_type text, entity_id text, action text, changes jsonb,
    ip_address text, user_agent text, created_at text, old_values jsonb, new_values jsonb)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    entity_type = EXCLUDED.entity_type,
    entity_id = EXCLUDED.entity_id,
    action = EXCLUDED.action,
    changes = EXCLUDED.changes,
    ip_address = EXCLUDED.ip_address,
    user_agent = EXCLUDED.user_agent,
    created_at = EXCLUDED.created_at,
    old_values = EXCLUDED.old_values,
    new_values = EXCLUDED.new_values;

-- name: ExportNotificationsRows :many
SELECT row_to_json(t) FROM notifications t ORDER BY t.id;

-- name: UpsertNotificationsRow :exec
INSERT INTO notifications (id, user_id, type, title, message, link, is_read, created_at)
SELECT r.id::uuid, r.user_id::uuid, r.type, r.title, r.message, r.link,
       r.is_read::boolean, r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, user_id text, type text, title text, message text, link text, is_read text,
    created_at text)
ON CONFLICT (id) DO UPDATE SET
    user_id = EXCLUDED.user_id,
    type = EXCLUDED.type,
    title = EXCLUDED.title,
    message = EXCLUDED.message,
    link = EXCLUDED.link,
    is_read = EXCLUDED.is_read,
    created_at = EXCLUDED.created_at;

-- name: ExportEmailMessagesRows :many
SELECT row_to_json(t) FROM email_messages t ORDER BY t.id;

-- name: UpsertEmailMessagesRow :exec
INSERT INTO email_messages (id, account_id, thread_id, message_id, in_reply_to, direction,
                            sender_email, sender_name, recipient_emails, subject, body_text,
                            body_html, received_at, is_read, contact_id, deal_id, created_at,
                            tags, is_draft)
SELECT r.id::uuid, r.account_id::uuid, r.thread_id, r.message_id, r.in_reply_to,
       r.direction::public.email_direction, r.sender_email, r.sender_name, r.recipient_emails,
       r.subject, r.body_text, r.body_html, r.received_at::timestamptz, r.is_read::boolean,
       r.contact_id::uuid, r.deal_id::uuid, r.created_at::timestamptz, r.tags, r.is_draft::boolean
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, account_id text, thread_id text, message_id text, in_reply_to text, direction text,
    sender_email text, sender_name text, recipient_emails jsonb, subject text, body_text text,
    body_html text, received_at text, is_read text, contact_id text, deal_id text,
    created_at text, tags jsonb, is_draft text)
ON CONFLICT (id) DO UPDATE SET
    account_id = EXCLUDED.account_id,
    thread_id = EXCLUDED.thread_id,
    message_id = EXCLUDED.message_id,
    in_reply_to = EXCLUDED.in_reply_to,
    direction = EXCLUDED.direction,
    sender_email = EXCLUDED.sender_email,
    sender_name = EXCLUDED.sender_name,
    recipient_emails = EXCLUDED.recipient_emails,
    subject = EXCLUDED.subject,
    body_text = EXCLUDED.body_text,
    body_html = EXCLUDED.body_html,
    received_at = EXCLUDED.received_at,
    is_read = EXCLUDED.is_read,
    contact_id = EXCLUDED.contact_id,
    deal_id = EXCLUDED.deal_id,
    created_at = EXCLUDED.created_at,
    tags = EXCLUDED.tags,
    is_draft = EXCLUDED.is_draft;

-- name: ExportEmailAttachmentsRows :many
SELECT row_to_json(t) FROM email_attachments t ORDER BY t.id;

-- name: UpsertEmailAttachmentsRow :exec
INSERT INTO email_attachments (id, message_id, filename, content_type, size_bytes, storage_path, created_at)
SELECT r.id::uuid, r.message_id::uuid, r.filename, r.content_type, r.size_bytes::bigint,
       r.storage_path, r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, message_id text, filename text, content_type text, size_bytes text,
    storage_path text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    message_id = EXCLUDED.message_id,
    filename = EXCLUDED.filename,
    content_type = EXCLUDED.content_type,
    size_bytes = EXCLUDED.size_bytes,
    storage_path = EXCLUDED.storage_path,
    created_at = EXCLUDED.created_at;

-- name: ExportAppointmentsRows :many
SELECT row_to_json(t) FROM appointments t ORDER BY t.id;

-- name: UpsertAppointmentsRow :exec
INSERT INTO appointments (id, title, contact_id, company_id, deal_id, start_time, end_time,
                          location, notes, ics_uid, created_at, assigned_to, type, is_external,
                          provider, is_private, is_pushed, updated_at)
SELECT r.id::uuid, r.title, r.contact_id::uuid, r.company_id::uuid, r.deal_id::uuid,
       r.start_time::timestamptz, r.end_time::timestamptz, r.location, r.notes, r.ics_uid,
       r.created_at::timestamptz, r.assigned_to, r.type, r.is_external::boolean, r.provider,
       r.is_private::boolean, r.is_pushed::boolean, r.updated_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, title text, contact_id text, company_id text, deal_id text, start_time text,
    end_time text, location text, notes text, ics_uid text, created_at text, assigned_to text,
    type text, is_external text, provider text, is_private text, is_pushed text, updated_at text)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    contact_id = EXCLUDED.contact_id,
    company_id = EXCLUDED.company_id,
    deal_id = EXCLUDED.deal_id,
    start_time = EXCLUDED.start_time,
    end_time = EXCLUDED.end_time,
    location = EXCLUDED.location,
    notes = EXCLUDED.notes,
    ics_uid = EXCLUDED.ics_uid,
    created_at = EXCLUDED.created_at,
    assigned_to = EXCLUDED.assigned_to,
    type = EXCLUDED.type,
    is_external = EXCLUDED.is_external,
    provider = EXCLUDED.provider,
    is_private = EXCLUDED.is_private,
    is_pushed = EXCLUDED.is_pushed,
    updated_at = EXCLUDED.updated_at;

-- name: ExportCallActivitiesRows :many
SELECT row_to_json(t) FROM call_activities t ORDER BY t.id;

-- name: UpsertCallActivitiesRow :exec
INSERT INTO call_activities (id, contact_id, duration_seconds, disposition, notes, created_at)
SELECT r.id::uuid, r.contact_id::uuid, r.duration_seconds::integer, r.disposition, r.notes,
       r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, contact_id text, duration_seconds text, disposition text, notes text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    contact_id = EXCLUDED.contact_id,
    duration_seconds = EXCLUDED.duration_seconds,
    disposition = EXCLUDED.disposition,
    notes = EXCLUDED.notes,
    created_at = EXCLUDED.created_at;

-- name: ExportCompanyResearchRows :many
SELECT row_to_json(t) FROM company_research t ORDER BY t.id;

-- name: UpsertCompanyResearchRow :exec
INSERT INTO company_research (id, company_id, domain, title, meta_description, summary,
                              industry_keywords, researched_at)
SELECT r.id::uuid, r.company_id::uuid, r.domain, r.title, r.meta_description, r.summary,
       ARRAY(SELECT jsonb_array_elements_text(COALESCE(r.industry_keywords, '[]'::jsonb))),
       r.researched_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, company_id text, domain text, title text, meta_description text, summary text,
    industry_keywords jsonb, researched_at text)
ON CONFLICT (id) DO UPDATE SET
    company_id = EXCLUDED.company_id,
    domain = EXCLUDED.domain,
    title = EXCLUDED.title,
    meta_description = EXCLUDED.meta_description,
    summary = EXCLUDED.summary,
    industry_keywords = EXCLUDED.industry_keywords,
    researched_at = EXCLUDED.researched_at;

-- name: ExportWorkflowRunsRows :many
SELECT row_to_json(t) FROM workflow_runs t ORDER BY t.id;

-- name: UpsertWorkflowRunsRow :exec
INSERT INTO workflow_runs (id, workflow_id, target_id, target_type, target_name, status,
                           current_step, snapshot_json, started_at, completed_at)
SELECT r.id, r.workflow_id, r.target_id, r.target_type, r.target_name, r.status,
       r.current_step::integer, r.snapshot_json, r.started_at::timestamptz,
       r.completed_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, workflow_id text, target_id text, target_type text, target_name text, status text,
    current_step text, snapshot_json jsonb, started_at text, completed_at text)
ON CONFLICT (id) DO UPDATE SET
    workflow_id = EXCLUDED.workflow_id,
    target_id = EXCLUDED.target_id,
    target_type = EXCLUDED.target_type,
    target_name = EXCLUDED.target_name,
    status = EXCLUDED.status,
    current_step = EXCLUDED.current_step,
    snapshot_json = EXCLUDED.snapshot_json,
    started_at = EXCLUDED.started_at,
    completed_at = EXCLUDED.completed_at;

-- name: ExportWorkflowStepsRows :many
SELECT row_to_json(t) FROM workflow_steps t ORDER BY t.id;

-- name: UpsertWorkflowStepsRow :exec
INSERT INTO workflow_steps (id, run_id, step_number, title, action_type, payload_json, status,
                            prepared_at, executed_at)
SELECT r.id, r.run_id, r.step_number::integer, r.title, r.action_type, r.payload_json, r.status,
       r.prepared_at::timestamptz, r.executed_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, run_id text, step_number text, title text, action_type text, payload_json jsonb,
    status text, prepared_at text, executed_at text)
ON CONFLICT (id) DO UPDATE SET
    run_id = EXCLUDED.run_id,
    step_number = EXCLUDED.step_number,
    title = EXCLUDED.title,
    action_type = EXCLUDED.action_type,
    payload_json = EXCLUDED.payload_json,
    status = EXCLUDED.status,
    prepared_at = EXCLUDED.prepared_at,
    executed_at = EXCLUDED.executed_at;

-- name: ExportNotesRows :many
SELECT row_to_json(t) FROM notes t ORDER BY t.id;

-- name: UpsertNotesRow :exec
INSERT INTO notes (id, entity_type, entity_id, type, author, content, created_at, updated_at)
SELECT r.id::uuid, r.entity_type, r.entity_id::uuid, r.type, r.author, r.content,
       r.created_at::timestamptz, r.updated_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, entity_type text, entity_id text, type text, author text, content text,
    created_at text, updated_at text)
ON CONFLICT (id) DO UPDATE SET
    entity_type = EXCLUDED.entity_type,
    entity_id = EXCLUDED.entity_id,
    type = EXCLUDED.type,
    author = EXCLUDED.author,
    content = EXCLUDED.content,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ExportIntakeFormsRows :many
SELECT row_to_json(t) FROM intake_forms t ORDER BY t.id;

-- name: UpsertIntakeFormsRow :exec
INSERT INTO intake_forms (id, name, slug, fields_json, is_active, created_at)
SELECT r.id::uuid, r.name, r.slug, r.fields_json, r.is_active::boolean, r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, name text, slug text, fields_json jsonb, is_active text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    fields_json = EXCLUDED.fields_json,
    is_active = EXCLUDED.is_active,
    created_at = EXCLUDED.created_at;

-- name: ExportKnowledgeBaseArticlesRows :many
SELECT row_to_json(t) FROM knowledge_base_articles t ORDER BY t.id;

-- name: UpsertKnowledgeBaseArticlesRow :exec
INSERT INTO knowledge_base_articles (id, title, category, content, tags, author, created_at,
                                     updated_at, embedding, embedding_model)
SELECT r.id::uuid, r.title, r.category, r.content,
       ARRAY(SELECT jsonb_array_elements_text(COALESCE(r.tags, '[]'::jsonb))),
       r.author, r.created_at::timestamptz, r.updated_at::timestamptz,
       (r.embedding #>> '{}')::public.vector(1024), r.embedding_model
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, title text, category text, content text, tags jsonb, author text,
    created_at text, updated_at text, embedding jsonb, embedding_model text)
ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    category = EXCLUDED.category,
    content = EXCLUDED.content,
    tags = EXCLUDED.tags,
    author = EXCLUDED.author,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    embedding = EXCLUDED.embedding,
    embedding_model = EXCLUDED.embedding_model;

-- name: ExportAIResearchJobsRows :many
SELECT row_to_json(t) FROM ai_research_jobs t ORDER BY t.id;

-- name: UpsertAIResearchJobsRow :exec
INSERT INTO ai_research_jobs (id, company_name, domain, status, result_json, error_message,
                              created_at, completed_at)
SELECT r.id::uuid, r.company_name, r.domain, r.status, r.result_json, r.error_message,
       r.created_at::timestamptz, r.completed_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, company_name text, domain text, status text, result_json jsonb,
    error_message text, created_at text, completed_at text)
ON CONFLICT (id) DO UPDATE SET
    company_name = EXCLUDED.company_name,
    domain = EXCLUDED.domain,
    status = EXCLUDED.status,
    result_json = EXCLUDED.result_json,
    error_message = EXCLUDED.error_message,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    completed_at = EXCLUDED.completed_at;

-- name: ExportAIAuditLogsRows :many
SELECT row_to_json(t) FROM ai_audit_logs t ORDER BY t.id;

-- name: UpsertAIAuditLogsRow :exec
INSERT INTO ai_audit_logs (id, interaction_type, model_name, provider, prompt_tokens,
                           completion_tokens, latency_ms, pii_filter_triggered,
                           pii_redactions_count, human_approved, created_at)
SELECT r.id::uuid, r.interaction_type, r.model_name, r.provider, r.prompt_tokens::integer,
       r.completion_tokens::integer, r.latency_ms::integer, r.pii_filter_triggered::boolean,
       r.pii_redactions_count::integer, r.human_approved::boolean, r.created_at::timestamptz
FROM jsonb_to_record(sqlc.arg(row)::jsonb) AS r(
    id text, interaction_type text, model_name text, provider text, prompt_tokens text,
    completion_tokens text, latency_ms text, pii_filter_triggered text,
    pii_redactions_count text, human_approved text, created_at text)
ON CONFLICT (id) DO UPDATE SET
    interaction_type = EXCLUDED.interaction_type,
    model_name = EXCLUDED.model_name,
    provider = EXCLUDED.provider,
    prompt_tokens = EXCLUDED.prompt_tokens,
    completion_tokens = EXCLUDED.completion_tokens,
    latency_ms = EXCLUDED.latency_ms,
    pii_filter_triggered = EXCLUDED.pii_filter_triggered,
    pii_redactions_count = EXCLUDED.pii_redactions_count,
    human_approved = EXCLUDED.human_approved,
    created_at = EXCLUDED.created_at;

-- name: TruncateBackupTables :exec
TRUNCATE TABLE
    ai_audit_logs,
    ai_research_jobs,
    knowledge_base_articles,
    email_templates,
    user_invites,
    intake_forms,
    notes,
    workflow_steps,
    workflow_runs,
    company_research,
    call_activities,
    appointments,
    email_attachments,
    email_messages,
    notifications,
    audit_logs,
    refresh_tokens,
    todos,
    email_accounts,
    deals,
    contacts,
    workflows,
    companies,
    users
RESTART IDENTITY CASCADE;
