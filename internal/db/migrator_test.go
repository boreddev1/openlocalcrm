package db

import (
	"testing"
)

func TestGetEmbeddedMigrations(t *testing.T) {
	migrations, err := GetEmbeddedMigrations()
	if err != nil {
		t.Fatalf("unexpected error getting embedded migrations: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatalf("expected at least 1 embedded migration")
	}

	if migrations[0] != "00001_initial_schema.sql" {
		t.Errorf("expected first migration to be 00001_initial_schema.sql, got %s", migrations[0])
	}

	latest := GetLatestEmbeddedMigration()
	if latest == "" {
		t.Errorf("expected non-empty latest migration")
	}
}
