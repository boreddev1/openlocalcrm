package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// GetEmbeddedMigrations returns the list of all embedded SQL migration filenames sorted chronologically.
func GetEmbeddedMigrations() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			filenames = append(filenames, entry.Name())
		}
	}
	sort.Strings(filenames)
	return filenames, nil
}

// GetLatestEmbeddedMigration returns the filename of the newest migration known to this application version.
func GetLatestEmbeddedMigration() string {
	filenames, err := GetEmbeddedMigrations()
	if err != nil || len(filenames) == 0 {
		return ""
	}
	return filenames[len(filenames)-1]
}

// MigrationStatus reports the current status of applied and pending migrations.
type MigrationStatus struct {
	CurrentVersion  string   `json:"current_version"`
	AppliedVersions []string `json:"applied_versions"`
	PendingVersions []string `json:"pending_versions"`
	FutureVersions  []string `json:"future_versions"`
	IsUpToDate      bool     `json:"is_up_to_date"`
}

// GetMigrationStatus checks the database against embedded migrations.
func GetMigrationStatus(ctx context.Context, pool *pgxpool.Pool) (MigrationStatus, error) {
	status := MigrationStatus{
		AppliedVersions: make([]string, 0),
		PendingVersions: make([]string, 0),
		FutureVersions:  make([]string, 0),
	}

	filenames, err := GetEmbeddedMigrations()
	if err != nil {
		return status, err
	}

	knownMap := make(map[string]bool)
	for _, fn := range filenames {
		knownMap[fn] = true
	}

	// 1. Fetch applied versions
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version ASC")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err == nil {
				status.AppliedVersions = append(status.AppliedVersions, v)
				if !knownMap[v] {
					status.FutureVersions = append(status.FutureVersions, v)
				}
			}
		}
	}

	if len(status.AppliedVersions) > 0 {
		status.CurrentVersion = status.AppliedVersions[len(status.AppliedVersions)-1]
	}

	// 2. Identify pending versions
	appliedMap := make(map[string]bool)
	for _, v := range status.AppliedVersions {
		appliedMap[v] = true
	}

	for _, fn := range filenames {
		if !appliedMap[fn] {
			status.PendingVersions = append(status.PendingVersions, fn)
		}
	}

	status.IsUpToDate = len(status.PendingVersions) == 0 && len(status.FutureVersions) == 0
	return status, nil
}

// RunMigrations applies all SQL migrations in chronological order.
// If an older backup was restored, this automatically applies all intermediate migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// 1. Ensure migrations tracking table exists
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// 2. Read embedded migration files
	filenames, err := GetEmbeddedMigrations()
	if err != nil {
		return err
	}

	// 3. Apply pending migrations
	appliedCount := 0
	for _, filename := range filenames {
		var exists bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", filename).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed checking migration status for %s: %w", filename, err)
		}
		if exists {
			continue
		}

		content, err := fs.ReadFile(migrationsFS, "migrations/"+filename)
		if err != nil {
			return fmt.Errorf("failed reading migration file %s: %w", filename, err)
		}

		sqlContent := string(content)
		if idx := strings.Index(sqlContent, "-- +goose Down"); idx != -1 {
			sqlContent = sqlContent[:idx]
		}

		log.Printf("[DB Migrator] Forward-migrating: applying %s...", filename)
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed starting tx for %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx, sqlContent); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed executing migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", filename); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed recording migration %s: %w", filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed committing migration %s: %w", filename, err)
		}
		appliedCount++
		log.Printf("[DB Migrator] Successfully applied migration: %s", filename)
	}

	if appliedCount > 0 {
		log.Printf("[DB Migrator] Completed forward-migration (%d new migration(s) applied)", appliedCount)
	}

	// 4. Check if database has any future migrations that are unknown to this application
	status, _ := GetMigrationStatus(ctx, pool)
	if len(status.FutureVersions) > 0 {
		log.Printf("==================================================================")
		log.Printf("[DB Migrator] ⚠️ WARNING: RESTORED DATABASE CONTAINS FUTURE MIGRATIONS:")
		for _, fv := range status.FutureVersions {
			log.Printf("  - %s (unknown to this version %s)", fv, GetLatestEmbeddedMigration())
		}
		log.Printf("Please update OpenLocalCRM to the latest version to prevent incompatibilities.")
		log.Printf("==================================================================")
	}

	return nil
}
