package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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

// TestRefreshRotationAgainstRealPostgres proves the rotation transaction, the
// replay path and the version bump against a real PostgreSQL database.
func TestRefreshRotationAgainstRealPostgres(t *testing.T) {
	url := postgresURL(t)
	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()

	ctx := context.Background()
	queries := db.New(pool)

	// Seed test user
	userID := uuid.New()
	userEmail := fmt.Sprintf("session-pg-%d@example.de", time.Now().UnixNano())
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, first_name, last_name, role, status, token_version)
		VALUES ($1, $2, 'hash', 'Session', 'Tester', 'VERTRIEB', 'ACTIVE', 1)
	`, userID, userEmail)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	hashToken := func(raw string) string {
		h := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(h[:])
	}

	familyID := uuid.New()
	rawFirst := fmt.Sprintf("raw-token-first-%d", time.Now().UnixNano())
	firstHash := hashToken(rawFirst)

	_, err = queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		TokenHash: firstHash,
		UserAgent: pgtype.Text{String: "test-agent", Valid: true},
		IpAddress: pgtype.Text{String: "127.0.0.1", Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
		FamilyID:  pgtype.UUID{Bytes: familyID, Valid: true},
	})
	require.NoError(t, err)

	// 1. First consume succeeds
	consumed, err := queries.ConsumeRefreshToken(ctx, firstHash)
	require.NoError(t, err)
	require.Equal(t, familyID, uuid.UUID(consumed.FamilyID.Bytes))

	// 2. Second consume of the same hash returns no row (replay detection signal)
	_, err = queries.ConsumeRefreshToken(ctx, firstHash)
	require.ErrorIs(t, err, pgx.ErrNoRows, "a spent refresh token must not be consumable a second time")

	// 3. Concurrent consume of a fresh token yields exactly one winner
	rawSecond := fmt.Sprintf("raw-token-second-%d", time.Now().UnixNano())
	secondHash := hashToken(rawSecond)
	familyID2 := uuid.New()

	_, err = queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		TokenHash: secondHash,
		UserAgent: pgtype.Text{String: "test-agent", Valid: true},
		IpAddress: pgtype.Text{String: "127.0.0.1", Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
		FamilyID:  pgtype.UUID{Bytes: familyID2, Valid: true},
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			_, consumeErr := queries.ConsumeRefreshToken(context.Background(), secondHash)
			results[idx] = consumeErr
		}()
	}
	wg.Wait()

	var successes int
	for _, resErr := range results {
		if resErr == nil {
			successes++
		} else {
			require.ErrorIs(t, resErr, pgx.ErrNoRows)
		}
	}
	require.Equal(t, 1, successes, "concurrent consume must produce exactly one successful rotation")

	// 4. Incrementing token_version invalidates issued access tokens
	beforeVersion, err := queries.CurrentTokenVersion(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	require.NoError(t, err)

	_, err = queries.IncrementUserTokenVersion(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	require.NoError(t, err)

	afterVersion, err := queries.CurrentTokenVersion(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	require.NoError(t, err)
	require.Equal(t, beforeVersion+1, afterVersion)
}
