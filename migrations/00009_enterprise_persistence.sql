-- Migration 00009: Enterprise Persistence (Appointments, Knowledge Base, AI Research Jobs)

-- 1. Extend appointments table with enterprise scheduling & synchronization columns
ALTER TABLE appointments 
    ADD COLUMN IF NOT EXISTS assigned_to VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS type VARCHAR(64) NOT NULL DEFAULT 'CONSULTATION',
    ADD COLUMN IF NOT EXISTS is_external BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS provider VARCHAR(64) NOT NULL DEFAULT 'internal',
    ADD COLUMN IF NOT EXISTS is_private BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_pushed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_appointments_assigned_to ON appointments(assigned_to);
CREATE INDEX IF NOT EXISTS idx_appointments_contact_id ON appointments(contact_id);

-- 2. Knowledge Base articles for Enterprise RAG
CREATE TABLE IF NOT EXISTS knowledge_base_articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    category VARCHAR(128) NOT NULL,
    content TEXT NOT NULL,
    tags TEXT[] NOT NULL DEFAULT '{}',
    author VARCHAR(255) NOT NULL DEFAULT 'System',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_category ON knowledge_base_articles(category);
CREATE INDEX IF NOT EXISTS idx_kb_created_at ON knowledge_base_articles(created_at DESC);

-- 3. AI Research Jobs persistence
CREATE TABLE IF NOT EXISTS ai_research_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(64) NOT NULL DEFAULT 'PENDING',
    result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ai_research_jobs_created_at ON ai_research_jobs(created_at DESC);
