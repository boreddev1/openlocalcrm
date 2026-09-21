-- +goose Up
-- Foreign Key Indexes for query performance
-- Befund 21: Fehlende Fremdschlüssel-Indizes

-- Contacts → Companies
CREATE INDEX IF NOT EXISTS idx_contacts_company_id ON contacts (company_id) WHERE company_id IS NOT NULL;

-- Deals → Contacts, Companies, Users
CREATE INDEX IF NOT EXISTS idx_deals_contact_id ON deals (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_deals_company_id ON deals (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_deals_assigned_to ON deals (assigned_to) WHERE assigned_to IS NOT NULL;

-- Todos → Users
CREATE INDEX IF NOT EXISTS idx_todos_assigned_to ON todos (assigned_to) WHERE assigned_to IS NOT NULL;

-- Notes → Contacts
CREATE INDEX IF NOT EXISTS idx_notes_contact_id ON notes (contact_id) WHERE contact_id IS NOT NULL;

-- Appointments → Contacts, Companies, Deals
CREATE INDEX IF NOT EXISTS idx_appointments_contact_id ON appointments (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_company_id ON appointments (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_deal_id ON appointments (deal_id) WHERE deal_id IS NOT NULL;

-- Email Messages → Accounts, Contacts
CREATE INDEX IF NOT EXISTS idx_email_messages_account_id ON email_messages (account_id);
CREATE INDEX IF NOT EXISTS idx_email_messages_contact_id ON email_messages (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_email_messages_thread_id ON email_messages (thread_id);

-- Workflow Runs → Workflows
CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_id ON workflow_runs (workflow_id);

-- Audit Logs → Entity lookups
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs (entity_type, entity_id);

-- AI Audit Logs → Timestamp lookups
CREATE INDEX IF NOT EXISTS idx_ai_audit_logs_created ON ai_audit_logs (created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_contacts_company_id;
DROP INDEX IF EXISTS idx_deals_contact_id;
DROP INDEX IF EXISTS idx_deals_company_id;
DROP INDEX IF EXISTS idx_deals_assigned_to;
DROP INDEX IF EXISTS idx_todos_assigned_to;
DROP INDEX IF EXISTS idx_notes_contact_id;
DROP INDEX IF EXISTS idx_appointments_contact_id;
DROP INDEX IF EXISTS idx_appointments_company_id;
DROP INDEX IF EXISTS idx_appointments_deal_id;
DROP INDEX IF EXISTS idx_email_messages_account_id;
DROP INDEX IF EXISTS idx_email_messages_contact_id;
DROP INDEX IF EXISTS idx_email_messages_thread_id;
DROP INDEX IF EXISTS idx_workflow_runs_workflow_id;
DROP INDEX IF EXISTS idx_audit_logs_entity;
DROP INDEX IF EXISTS idx_ai_audit_logs_created;
