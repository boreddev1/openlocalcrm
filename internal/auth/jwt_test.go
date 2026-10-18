package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

	tokenStr, err := auth.GenerateAccessToken(userID, email, role, 0, privKey, 15*time.Minute)
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

var testUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func testKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key pair: %v", err)
	}
	return priv, pub
}

// TestJWTTokenVersionIsRequired is the regression guard for the upgrade path: a
// token minted before SP8a has no token_version claim, and defaulting it to 0
// would let it pass the version check until it expired.
func TestJWTTokenVersionIsRequired(t *testing.T) {
	priv, pub := testKeyPair(t)

	withVersion, err := auth.GenerateAccessToken(testUserID, "a@example.de", "ADMIN", 7, priv, time.Minute)
	if err != nil {
		t.Fatalf("expected token generation to succeed: %v", err)
	}
	claims, err := auth.ValidateAccessToken(withVersion, pub)
	if err != nil {
		t.Fatalf("expected validation to succeed: %v", err)
	}
	if claims.TokenVersion == nil || *claims.TokenVersion != 7 {
		t.Fatalf("expected token version 7, got %v", claims.TokenVersion)
	}

	// Sign the same claims without the claim, exactly as the pre-SP8a code did.
	legacy := auth.AccessClaims{
		UserID: testUserID,
		Email:  "a@example.de",
		Role:   "ADMIN",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   testUserID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Minute)),
			Issuer:    "openlocalcrm",
			Audience:  jwt.ClaimStrings{"openlocalcrm-api"},
		},
	}
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, legacy).SignedString(priv)
	if err != nil {
		t.Fatalf("signing legacy token failed: %v", err)
	}

	_, err = auth.ValidateAccessToken(legacyToken, pub)
	if err == nil {
		t.Fatalf("a token without token_version must be rejected")
	}
}
