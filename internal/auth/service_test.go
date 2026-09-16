package auth_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/pquerna/otp/totp"
)

func setupTestAuthService(t *testing.T) (*auth.AuthService, *demo.InMemoryQuerier, ed25519.PublicKey) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keys: %v", err)
	}
	querier := demo.NewInMemoryQuerier()
	limiter := auth.NewRateLimiter(3, time.Minute, time.Minute)
	svc := auth.NewAuthService(querier, privKey, pubKey, limiter)
	return svc, querier, pubKey
}

func TestAuthService_LoginSuccessAndFail(t *testing.T) {
	svc, _, pubKey := setupTestAuthService(t)
	ctx := context.Background()

	// Successful login
	res, err := svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("expected login success, got error: %v", err)
	}
	if res.Token == "" || res.RefreshToken == "" {
		t.Fatalf("expected token and refresh token")
	}

	claims, err := auth.ValidateAccessToken(res.Token, pubKey)
	if err != nil || claims.Email != "admin@openlocalcrm.local" {
		t.Fatalf("invalid claims in token: %v", err)
	}

	// Failed login (wrong password)
	_, err = svc.Login(ctx, "admin@openlocalcrm.local", "wrongpassword", "", "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected login failure for wrong password")
	}

	// Failed login (non-existent email)
	_, err = svc.Login(ctx, "nonexistent@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected login failure for non-existent user")
	}
}

func TestAuthService_RefreshToken(t *testing.T) {
	svc, _, pubKey := setupTestAuthService(t)
	ctx := context.Background()

	res, err := svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	newToken, err := svc.RefreshToken(ctx, res.RefreshToken, "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	claims, err := auth.ValidateAccessToken(newToken, pubKey)
	if err != nil || claims.Email != "admin@openlocalcrm.local" {
		t.Fatalf("invalid refreshed token: %v", err)
	}

	// Invalid refresh token
	_, err = svc.RefreshToken(ctx, "invalid-token", "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected error for invalid refresh token")
	}
}

func TestAuthService_TOTPFlow(t *testing.T) {
	svc, querier, _ := setupTestAuthService(t)
	ctx := context.Background()

	admin, _ := querier.GetUserByEmail(ctx, "admin@openlocalcrm.local")
	adminUUID := uuid.UUID(admin.ID.Bytes)

	// Setup TOTP
	setup, err := svc.SetupTOTP(ctx, adminUUID)
	if err != nil {
		t.Fatalf("setup TOTP failed: %v", err)
	}
	if setup.Secret == "" || setup.URL == "" {
		t.Fatalf("missing secret or url in TOTP setup")
	}

	// Generate valid code
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("failed generating totp code: %v", err)
	}

	// Verify TOTP
	err = svc.VerifyTOTP(ctx, adminUUID, code)
	if err != nil {
		t.Fatalf("verify TOTP failed: %v", err)
	}

	// Now login should require TOTP!
	loginRes, err := svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loginRes.TOTPRequired {
		t.Fatalf("expected TOTPRequired to be true")
	}

	// Login with valid TOTP code
	codeNow, _ := totp.GenerateCode(setup.Secret, time.Now())
	loginRes, err = svc.Login(ctx, "admin@openlocalcrm.local", "demo123", codeNow, "127.0.0.1", "TestAgent")
	if err != nil || loginRes.Token == "" {
		t.Fatalf("expected login success with TOTP, got err: %v", err)
	}

	// Disable TOTP
	err = svc.DisableTOTP(ctx, adminUUID, "demo123")
	if err != nil {
		t.Fatalf("disable TOTP failed: %v", err)
	}

	// Login without TOTP should work again
	loginRes, err = svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err != nil || loginRes.Token == "" || loginRes.TOTPRequired {
		t.Fatalf("expected direct login success after disabling TOTP")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	svc, querier, _ := setupTestAuthService(t)
	ctx := context.Background()

	admin, _ := querier.GetUserByEmail(ctx, "admin@openlocalcrm.local")
	adminUUID := uuid.UUID(admin.ID.Bytes)

	// Short password should fail
	err := svc.ChangePassword(ctx, adminUUID, "demo123", "short")
	if err != auth.ErrPasswordTooShort {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}

	// Wrong old password should fail
	err = svc.ChangePassword(ctx, adminUUID, "wrongold", "newsecretpassword123")
	if err != auth.ErrInvalidOldPassword {
		t.Fatalf("expected ErrInvalidOldPassword, got %v", err)
	}

	// Successful password change
	err = svc.ChangePassword(ctx, adminUUID, "demo123", "newsecretpassword123")
	if err != nil {
		t.Fatalf("change password failed: %v", err)
	}

	// Old password should no longer work
	_, err = svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected login failure with old password")
	}

	// New password should work
	res, err := svc.Login(ctx, "admin@openlocalcrm.local", "newsecretpassword123", "", "127.0.0.1", "TestAgent")
	if err != nil || res.Token == "" {
		t.Fatalf("expected login success with new password, got: %v", err)
	}
}
