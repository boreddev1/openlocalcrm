package db_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/stretchr/testify/require"
)

// TestMigrationsReplayOnEmptyDatabase replays every embedded migration against a
// brand-new database, exactly like a first installation. The demo querier and
// per-feature Postgres tests never exercise this path.
func TestMigrationsReplayOnEmptyDatabase(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live Postgres test")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, adminURL)
	require.NoError(t, err)
	defer admin.Close()

	dbName := fmt.Sprintf("replay_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE DATABASE "+dbName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	})

	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	u.Path = "/" + dbName

	pool, err := pgxpool.New(ctx, u.String())
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, db.RunMigrations(ctx, pool))

	status, err := db.GetMigrationStatus(ctx, pool)
	require.NoError(t, err)
	require.True(t, status.IsUpToDate)
	require.Empty(t, status.PendingVersions)
	require.Equal(t, db.GetLatestEmbeddedMigration(), status.CurrentVersion)
}
