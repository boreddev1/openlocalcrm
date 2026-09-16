package auth_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestPasswordHashing(t *testing.T) {
	password := "SuperSecretPassword123!#"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("expected no error hashing password, got: %v", err)
	}

	if hash == "" {
		t.Fatalf("expected non-empty hash string")
	}

	if !auth.CheckPassword(hash, password) {
		t.Fatalf("expected password verification to succeed")
	}

	if auth.CheckPassword(hash, "WrongPassword456!") {
		t.Fatalf("expected password verification to fail for incorrect password")
	}

	if auth.CheckPassword("invalid$hash$string", password) {
		t.Fatalf("expected verification to fail for malformed hash")
	}
}
