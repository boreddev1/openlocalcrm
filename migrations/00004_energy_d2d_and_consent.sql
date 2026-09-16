-- migrations/00004_energy_d2d_and_consent.sql
-- +goose Up

-- Extend contacts table with Consent (UWG § 7) & Energy/D2D specifics (§3, §7)
ALTER TABLE contacts
    ADD COLUMN IF NOT EXISTS consent_phone BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS consent_email BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS consent_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS stromverbrauch_kwh NUMERIC(10, 2),
    ADD COLUMN IF NOT EXISTS gasverbrauch_kwh NUMERIC(10, 2),
    ADD COLUMN IF NOT EXISTS zaehlernummer VARCHAR(100),
    ADD COLUMN IF NOT EXISTS eigentuemer_status VARCHAR(50) DEFAULT 'UNBEKANNT';

-- Extend deals with Widerruf support (§ 355 BGB, §3.3)
ALTER TABLE deals
    ADD COLUMN IF NOT EXISTS widerrufen_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS widerruf_grund TEXT;

-- In-App Notification Center table (§14)
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL, -- 'EMAIL_RECEIVED', 'LEAD_ASSIGNED', 'TODO_DUE', 'WIDERRUF', 'SYSTEM'
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    link VARCHAR(255),
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id, is_read, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;
ALTER TABLE deals DROP COLUMN IF EXISTS widerruf_grund, DROP COLUMN IF EXISTS widerrufen_at;
ALTER TABLE contacts 
    DROP COLUMN IF EXISTS eigentuemer_status,
    DROP COLUMN IF EXISTS zaehlernummer,
    DROP COLUMN IF EXISTS gasverbrauch_kwh,
    DROP COLUMN IF EXISTS stromverbrauch_kwh,
    DROP COLUMN IF EXISTS consent_updated_at,
    DROP COLUMN IF EXISTS consent_email,
    DROP COLUMN IF EXISTS consent_phone;
