-- +goose Up
-- SP8a: durable session revocation, refresh token families, and stable
-- resource ownership. See docs/superpowers/specs/2026-09-26-sp8a-auth-sessions-rbac-design.md

-- Access tokens carry token_version; a mismatch with the stored value
-- invalidates every token issued before the change.
ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0;

-- Set for the bootstrapped admin. Soft: reported on /me, cleared on change.
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- Refresh rotation becomes single-use. family_id groups a rotation chain;
-- used_at marks a token as spent so a replay is detectable.
ALTER TABLE refresh_tokens ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_unused
    ON refresh_tokens(user_id) WHERE used_at IS NULL;

-- Ownership moves from free text to a stable user id. deals.assigned_to and
-- todos.assigned_to are already UUIDs and need no change.
ALTER TABLE contacts ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE companies ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE notes ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE appointments ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_contacts_owner_user_id ON contacts(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_companies_owner_user_id ON companies(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_notes_user_id ON notes(user_id);
CREATE INDEX IF NOT EXISTS idx_appointments_user_id ON appointments(user_id);

-- Backfill where a source exists: notes.author and appointments.assigned_to
-- hold the e-mail of the author or assignee. Rows that match no user stay NULL,
-- which under the ownership rule means ADMIN-only writes until assigned.
UPDATE notes SET user_id = u.id
  FROM users u
 WHERE lower(notes.author) = lower(u.email) AND notes.user_id IS NULL;

UPDATE appointments SET user_id = u.id
  FROM users u
 WHERE lower(appointments.assigned_to) = lower(u.email) AND appointments.user_id IS NULL;

-- +goose Down
ALTER TABLE appointments DROP COLUMN IF EXISTS user_id;
ALTER TABLE notes DROP COLUMN IF EXISTS user_id;
ALTER TABLE companies DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE contacts DROP COLUMN IF EXISTS owner_user_id;
DROP INDEX IF EXISTS idx_refresh_tokens_user_unused;
DROP INDEX IF EXISTS idx_refresh_tokens_family;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS used_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS family_id;
ALTER TABLE users DROP COLUMN IF EXISTS must_change_password;
ALTER TABLE users DROP COLUMN IF EXISTS token_version;
