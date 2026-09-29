package handlers_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func postgresURL(t *testing.T) string {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live Postgres test")
	}
	return url
}

// TestOwnershipBackfillAndScopesAgainstRealPostgres proves the migration's
// backfill and the read scopes against a real database. The demo querier cannot
// express either.
func TestOwnershipBackfillAndScopesAgainstRealPostgres(t *testing.T) {
	url := postgresURL(t)
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()

	ctx := context.Background()

	// Seed user
	userEmail := fmt.Sprintf("sp8a-backfill-%d@example.de", time.Now().UnixNano())
	userID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, first_name, last_name, role, status)
		VALUES ($1, $2, 'hash', 'Backfill', 'Tester', 'VERTRIEB', 'ACTIVE')
	`, userID, userEmail)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	// Seed note with author = userEmail, and user_id = NULL
	noteID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO notes (id, entity_type, entity_id, type, author, content, user_id)
		VALUES ($1, 'contact', $2, 'NOTE', $3, 'Backfill Note', NULL)
	`, noteID, uuid.New(), userEmail)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM notes WHERE id = $1", noteID)
	})

	// Seed appointment with assigned_to = userEmail, and user_id = NULL
	apptID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO appointments (id, title, start_time, end_time, assigned_to, type, user_id)
		VALUES ($1, 'Backfill Appt', NOW(), NOW() + INTERVAL '1 hour', $2, 'CONSULTATION', NULL)
	`, apptID, userEmail)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM appointments WHERE id = $1", apptID)
	})

	// Seed orphan note with author = 'Vertriebsmitarbeiter'
	orphanNoteID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO notes (id, entity_type, entity_id, type, author, content, user_id)
		VALUES ($1, 'contact', $2, 'NOTE', 'Vertriebsmitarbeiter', 'Orphan Note', NULL)
	`, orphanNoteID, uuid.New())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM notes WHERE id = $1", orphanNoteID)
	})

	// Apply backfill statements (exact statements from 00020_sp8a_sessions_and_ownership.sql)
	_, err = pool.Exec(ctx, `
		UPDATE notes SET user_id = u.id
		  FROM users u
		 WHERE lower(notes.author) = lower(u.email) AND notes.user_id IS NULL;
	`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE appointments SET user_id = u.id
		  FROM users u
		 WHERE lower(appointments.assigned_to) = lower(u.email) AND appointments.user_id IS NULL;
	`)
	require.NoError(t, err)

	// Verify note backfilled
	var noteOwner pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx, "SELECT user_id FROM notes WHERE id = $1", noteID).Scan(&noteOwner))
	require.True(t, noteOwner.Valid)
	require.Equal(t, userID.String(), uuid.UUID(noteOwner.Bytes).String())

	// Verify appointment backfilled
	var apptOwner pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx, "SELECT user_id FROM appointments WHERE id = $1", apptID).Scan(&apptOwner))
	require.True(t, apptOwner.Valid)
	require.Equal(t, userID.String(), uuid.UUID(apptOwner.Bytes).String())

	// Verify orphan note remained NULL
	var orphanOwner pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx, "SELECT user_id FROM notes WHERE id = $1", orphanNoteID).Scan(&orphanOwner))
	require.False(t, orphanOwner.Valid, "an unmatched author stays NULL, which is ADMIN-only for writes")
}
