-- Migration 00006: Calendar Appointments (§3.6), Telephony Activities (§7.1a), and Intake Forms (§6.5)

CREATE TABLE IF NOT EXISTS appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    contact_id UUID NULL REFERENCES contacts(id) ON DELETE SET NULL,
    company_id UUID NULL REFERENCES companies(id) ON DELETE SET NULL,
    deal_id UUID NULL REFERENCES deals(id) ON DELETE SET NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    location VARCHAR(255) NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    ics_uid VARCHAR(128) NOT NULL DEFAULT gen_random_uuid()::text,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS call_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contact_id UUID NULL REFERENCES contacts(id) ON DELETE CASCADE,
    duration_seconds INT NOT NULL DEFAULT 0,
    disposition VARCHAR(64) NOT NULL DEFAULT 'REACHED', -- 'REACHED', 'NO_ANSWER', 'BUSY', 'WRONG_NUMBER'
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS intake_forms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(128) NOT NULL UNIQUE,
    fields_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_appointments_start_time ON appointments(start_time);
CREATE INDEX IF NOT EXISTS idx_call_activities_contact_id ON call_activities(contact_id);
CREATE INDEX IF NOT EXISTS idx_intake_forms_slug ON intake_forms(slug);
