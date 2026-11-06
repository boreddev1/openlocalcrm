-- migrations/00016_automation_truth.sql
-- +goose Up

-- 1) Real tags columns for contacts and deals
ALTER TABLE contacts ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE deals ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 2) Email draft flag for DRAFT_EMAIL HITL flows
ALTER TABLE email_messages ADD COLUMN IF NOT EXISTS is_draft BOOLEAN NOT NULL DEFAULT FALSE;

-- 3) U5 Seeds: Default system workflows
INSERT INTO workflows (id, name, description, trigger_type, target_type, is_active, steps_json)
VALUES (
    'wf-1',
    'Erstkontakt & Qualifizierung (Neuer Lead)',
    'Automatische E-Mail-Triage bei Neukontakt, Tagging und Follow-up Aufgabe binnen 48 Stunden.',
    'NEW_LEAD',
    'CONTACT',
    true,
    '[{"step_number":1,"title":"Tag ''PV-Interessent'' zuweisen","action_type":"SET_TAG","payload":{"tag":"PV-Interessent"}},{"step_number":2,"title":"Erstkontakt E-Mail-Entwurf vorbereiten (Gemma 12B)","action_type":"DRAFT_EMAIL","payload":{"template":"onboarding_intro"}},{"step_number":3,"title":"Follow-Up Telefontermin anlegen","action_type":"CREATE_TASK","payload":{"due_days":2,"priority":"HIGH"}}]'::jsonb
),
(
    'wf-2',
    'Deal-Abschluss Routine (Phase: WON)',
    'Sendet Bestätigung, setzt Status auf Gewonnen und stößt Montage-Übergabe an.',
    'DEAL_WON',
    'DEAL',
    true,
    '[{"step_number":1,"title":"Auftragsbestätigung & Widerrufsbelehrung § 355 BGB senden","action_type":"DRAFT_EMAIL","payload":{"template":"order_confirmation"}},{"step_number":2,"title":"Übergabe-Task an Technik/Montage erstellen","action_type":"CREATE_TASK","payload":{"title":"Zählerkasten-Planung & Materialbestellung","priority":"URGENT"}}]'::jsonb
),
(
    'wf-3',
    'Inaktivitäts-Reaktivierung (SLA 30 Tage)',
    'Erinnert Vertriebler an Leads ohne Interaktion seit 30 Tagen.',
    'INACTIVITY_TIMEOUT',
    'CONTACT',
    true,
    '[{"step_number":1,"title":"Reaktivierungs-Todo für Account Manager erstellen","action_type":"CREATE_TASK","payload":{"title":"Lead-Nachfassen telefonisch","priority":"MEDIUM"}}]'::jsonb
)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM workflows WHERE id IN ('wf-1', 'wf-2', 'wf-3');
ALTER TABLE email_messages DROP COLUMN IF EXISTS is_draft;
ALTER TABLE deals DROP COLUMN IF EXISTS tags;
ALTER TABLE contacts DROP COLUMN IF EXISTS tags;
