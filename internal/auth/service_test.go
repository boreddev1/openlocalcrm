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

	newToken, newRefresh, err := svc.RefreshToken(ctx, res.RefreshToken, "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if newRefresh == "" {
		t.Fatalf("expected new refresh token")
	}
	claims, err := auth.ValidateAccessToken(newToken, pubKey)
	if err != nil || claims.Email != "admin@openlocalcrm.local" {
		t.Fatalf("invalid refreshed token: %v", err)
	}

	// Grace period test: Re-submitting old refresh token within 10s returns valid token pair
	graceToken, graceRefresh, err := svc.RefreshToken(ctx, res.RefreshToken, "127.0.0.1", "TestAgent")
	if err != nil || graceToken != newToken || graceRefresh != newRefresh {
		t.Fatalf("expected grace period cache hit, got err=%v, token=%s, refresh=%s", err, graceToken, graceRefresh)
	}

	// Invalid refresh token
	_, _, err = svc.RefreshToken(ctx, "invalid-token", "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected error for invalid refresh token")
	}

	// Token Revocation test
	auth.RevokeToken(newToken, time.Now().Add(1*time.Hour))
	if !auth.IsTokenRevoked(newToken) {
		t.Fatalf("expected token to be revoked")
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

	// Trying bypass passwords 'demo123' or 'oldpassword123' now that password is 'newsecretpassword123' MUST fail!
	err = svc.ChangePassword(ctx, adminUUID, "demo123", "anotherpassword123")
	if err != auth.ErrInvalidOldPassword {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY: demo123 bypass accepted in ChangePassword!")
	}
	err = svc.ChangePassword(ctx, adminUUID, "oldpassword123", "anotherpassword123")
	if err != auth.ErrInvalidOldPassword {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY: oldpassword123 bypass accepted in ChangePassword!")
	}
}

func TestLogin_ConstantTimeOnUnknownUser(t *testing.T) {
	svc, _, _ := setupTestAuthService(t)
	ctx := context.Background()

	// 1. Unknown user check duration: must perform real Argon2id derivation (> 20ms)
	startUnknown := time.Now()
	_, err := svc.Login(ctx, "unknown-user-does-not-exist@example.com", "any-password-123", "", "127.0.0.1", "TestAgent")
	durUnknown := time.Since(startUnknown)

	if err == nil {
		t.Fatalf("expected error for unknown user")
	}
	if durUnknown < 20*time.Millisecond {
		t.Fatalf("timing attack vulnerability (F-04): unknown user took only %v, expected > 20ms for full Argon2id calculation", durUnknown)
	}

	// 2. Wrong password check duration on real user
	startWrong := time.Now()
	_, err = svc.Login(ctx, "admin@openlocalcrm.local", "wrong-password-123", "", "127.0.0.1", "TestAgent")
	durWrong := time.Since(startWrong)

	if err == nil {
		t.Fatalf("expected error for wrong password")
	}
	if durWrong < 20*time.Millisecond {
		t.Fatalf("expected real user check to take > 20ms, took %v", durWrong)
	}
}
