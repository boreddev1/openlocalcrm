package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestJWTEd25519(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key pair: %v", err)
	}

	userID := uuid.New()
	email := "user@example.com"
	role := "ADMIN"

	tokenStr, err := auth.GenerateAccessToken(userID, email, role, privKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	claims, err := auth.ValidateAccessToken(tokenStr, pubKey)
	if err != nil {
		t.Fatalf("expected token validation to succeed, got: %v", err)
	}

	if claims.UserID != userID {
		t.Fatalf("expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != email {
		t.Fatalf("expected email %s, got %s", email, claims.Email)
	}
	if claims.Role != role {
		t.Fatalf("expected role %s, got %s", role, claims.Role)
	}

	// Test with invalid / expired key
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := auth.ValidateAccessToken(tokenStr, otherPub); err == nil {
		t.Fatalf("expected validation to fail with mismatched public key")
	}
}
