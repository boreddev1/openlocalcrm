-- Migration 00008: Customer Activity Notes (§3.4)
CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'NOTE',
    author VARCHAR(100) NOT NULL DEFAULT 'Vertriebsmitarbeiter',
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notes_entity ON notes(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at DESC);
