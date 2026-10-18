package queries_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
	"github.com/stretchr/testify/require"
)

// TestSP8aMigrationDeclaresTheSessionAndOwnershipColumns pins the migration
// contents. A column that is added to the schema but not to the backup contract
// is silently dropped on restore, so both sides are asserted here.
func TestSP8aMigrationDeclaresTheSessionAndOwnershipColumns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "00020_sp8a_sessions_and_ownership.sql"))
	require.NoError(t, err)
	migration := string(raw)

	for _, want := range []string{
		"ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE",
		"ALTER TABLE refresh_tokens ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid()",
		"ALTER TABLE refresh_tokens ADD COLUMN used_at TIMESTAMPTZ",
		"ALTER TABLE contacts ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE companies ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE notes ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"ALTER TABLE appointments ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE SET NULL",
	} {
		require.Contains(t, migration, want)
	}
	// Goose Down marker: the migrator only executes the text before it.
	require.Contains(t, migration, "-- +goose Down")
	// LF only, no CRLF.
	require.NotContains(t, migration, "\r")
}

// TestSP8aOwnershipColumnsSurviveBackupRestore fails if a new column is missing
// from the restore allow-list, which the restore validator enforces.
func TestSP8aOwnershipColumnsSurviveBackupRestore(t *testing.T) {
	for table, columns := range map[string][]string{
		"users":          {"token_version", "must_change_password"},
		"refresh_tokens": {"family_id", "used_at"},
		"contacts":       {"owner_user_id"},
		"companies":      {"owner_user_id"},
		"notes":          {"user_id"},
		"appointments":   {"user_id"},
	} {
		allow := backup.TableColumns(table)
		require.NotEmpty(t, allow, "table %s must be in the backup registry", table)
		for _, col := range columns {
			require.Contains(t, allow, col, "table %s is missing column %s in the restore allow-list", table, col)
		}
	}

	backupSQL, err := os.ReadFile(filepath.Join("..", "queries", "backup.sql"))
	require.NoError(t, err)
	for _, want := range []string{
		"r.token_version::integer",
		"r.must_change_password::boolean",
		"r.family_id::uuid",
		"r.used_at::timestamptz",
		"r.owner_user_id::uuid",
		"r.user_id::uuid",
	} {
		require.True(t, strings.Contains(string(backupSQL), want),
			"backup.sql must carry %q so restore does not drop the column", want)
	}
}
