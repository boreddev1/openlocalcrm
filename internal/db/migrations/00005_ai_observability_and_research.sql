-- Migration 00005: AI Observability (EU AI Act), Company Research, and AI Chat Session logs

CREATE TABLE IF NOT EXISTS ai_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interaction_type VARCHAR(64) NOT NULL, -- 'TRIAGE', 'RESEARCH', 'CHAT_COPILOT', 'DRAFT_REPLY'
    model_name VARCHAR(128) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    prompt_tokens INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    pii_filter_triggered BOOLEAN NOT NULL DEFAULT FALSE,
    pii_redactions_count INT NOT NULL DEFAULT 0,
    human_approved BOOLEAN NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS company_research (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id UUID NULL REFERENCES companies(id) ON DELETE CASCADE,
    domain VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL DEFAULT '',
    meta_description TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    industry_keywords TEXT[] NOT NULL DEFAULT '{}',
    researched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_audit_logs_created_at ON ai_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_company_research_domain ON company_research(domain);
