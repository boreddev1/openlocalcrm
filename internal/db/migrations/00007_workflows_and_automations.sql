-- Migrations: 00007_workflows_and_automations.sql
-- Workflows, Automationen & HITL Run Execution (§6.4)

CREATE TABLE IF NOT EXISTS workflows (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    trigger_type TEXT NOT NULL, -- 'NEW_LEAD', 'DEAL_WON', 'DEAL_STAGE_CHANGE', 'INBOUND_EMAIL', 'INACTIVITY_TIMEOUT', 'MANUAL'
    target_type TEXT NOT NULL,  -- 'CONTACT', 'COMPANY', 'DEAL'
    is_active BOOLEAN NOT NULL DEFAULT true,
    steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workflow_runs (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'IN_PROGRESS', -- 'IN_PROGRESS', 'WAITING_APPROVAL', 'COMPLETED', 'FAILED', 'CANCELLED'
    current_step INT NOT NULL DEFAULT 1,
    snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    step_number INT NOT NULL,
    title TEXT NOT NULL,
    action_type TEXT NOT NULL, -- 'DRAFT_EMAIL', 'CREATE_TASK', 'SET_TAG', 'NOTIFY_USER', 'WEBHOOK'
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'PREPARED', -- 'PREPARED', 'APPROVED', 'EXECUTED', 'SKIPPED'
    prepared_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    executed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_workflows_trigger ON workflows(trigger_type);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);
