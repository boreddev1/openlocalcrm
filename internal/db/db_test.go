package db_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

func TestGeneratedQuerierInterface(t *testing.T) {
	// Verify querier interface exists and models compile cleanly
	var _ db.Querier = (*db.Queries)(nil)
	user := db.User{
		Email:     "admin@example.com",
		FirstName: "Admin",
		LastName:  "User",
		Role:      "ADMIN",
		Status:    "ACTIVE",
	}
	if user.Role != "ADMIN" {
		t.Fatalf("expected role ADMIN, got %s", user.Role)
	}
}
