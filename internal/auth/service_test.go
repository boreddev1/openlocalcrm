package auth_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/crypto"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
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

	// Re-submitting old refresh token must fail because it was consumed (single-use)
	_, _, err = svc.RefreshToken(ctx, res.RefreshToken, "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("expected error for consumed refresh token")
	}

	// Invalid refresh token
	_, _, err = svc.RefreshToken(ctx, "invalid-token", "127.0.0.1", "TestAgent")
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
	err = svc.VerifyTOTP(ctx, adminUUID, setup.Secret, code)
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

	// Login with valid TOTP code for next time window
	codeNow, _ := totp.GenerateCode(setup.Secret, time.Now().Add(30*time.Second))
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

	// Verify bootstrap file removal and must_change_password flag reset
	tmpDir := t.TempDir()
	bootFile := filepath.Join(tmpDir, "bootstrap-admin.txt")
	require.NoError(t, os.WriteFile(bootFile, []byte("secret"), 0o600))
	svc.WithBootstrapSecretPath(bootFile)

	err = svc.ChangePassword(ctx, adminUUID, "newsecretpassword123", "brandnewsecret123")
	require.NoError(t, err)
	_, err = os.Stat(bootFile)
	require.True(t, os.IsNotExist(err), "bootstrap secret file must be deleted upon successful password change")

	updatedAdmin, err := querier.GetUserByID(ctx, admin.ID)
	require.NoError(t, err)
	require.False(t, updatedAdmin.MustChangePassword, "must_change_password must be false after password change")
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

func TestTOTP_SecretIsEncryptedInDatabase(t *testing.T) {
	svc, querier, _ := setupTestAuthService(t)
	ctx := context.Background()

	adminUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	setupRes, err := svc.SetupTOTP(ctx, adminUUID)
	if err != nil {
		t.Fatalf("SetupTOTP failed: %v", err)
	}

	// Before verify, nothing must be stored in DB (F-03)
	user, err := querier.GetUserByID(ctx, pgtype.UUID{Bytes: adminUUID, Valid: true})
	if err != nil {
		t.Fatalf("failed getting user: %v", err)
	}
	if user.TotpSecretEncrypted.Valid && user.TotpSecretEncrypted.String != "" {
		t.Fatalf("expected no stored secret in db before verification (F-03)")
	}

	code, err := totp.GenerateCode(setupRes.Secret, time.Now())
	if err != nil {
		t.Fatalf("failed generating totp code: %v", err)
	}
	if err := svc.VerifyTOTP(ctx, adminUUID, setupRes.Secret, code); err != nil {
		t.Fatalf("VerifyTOTP failed: %v", err)
	}

	user, err = querier.GetUserByID(ctx, pgtype.UUID{Bytes: adminUUID, Valid: true})
	if err != nil {
		t.Fatalf("failed getting user: %v", err)
	}

	storedSecret := user.TotpSecretEncrypted.String
	if storedSecret == "" {
		t.Fatalf("expected stored secret in db after verification")
	}

	// 1. Must NOT equal plaintext secret
	if storedSecret == setupRes.Secret {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY (F-01): TOTP secret stored in plaintext! stored=%s, plain=%s", storedSecret, setupRes.Secret)
	}

	// 2. Decrypting with crypto.DecryptSecret yields the plaintext setupRes.Secret
	decrypted, err := crypto.DecryptSecret(storedSecret)
	if err != nil {
		t.Fatalf("failed to decrypt stored secret: %v", err)
	}
	if decrypted != setupRes.Secret {
		t.Fatalf("expected decrypted secret to match plaintext setup secret, got %s != %s", decrypted, setupRes.Secret)
	}
}

func TestAuthService_TOTPReplayPrevention(t *testing.T) {
	svc, _, _ := setupTestAuthService(t)
	ctx := context.Background()
	adminUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Setup TOTP
	setupRes, err := svc.SetupTOTP(ctx, adminUUID)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	code, err := totp.GenerateCode(setupRes.Secret, time.Now())
	if err != nil {
		t.Fatalf("failed generating code: %v", err)
	}

	// Verify TOTP to enable it
	if err := svc.VerifyTOTP(ctx, adminUUID, setupRes.Secret, code); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Replay of the code used in VerifyTOTP must immediately fail on Login!
	_, err = svc.Login(ctx, "admin@openlocalcrm.local", "demo123", code, "127.0.0.1", "TestAgent")
	if err == nil {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY (F-02): expected replay of TOTP verification code to fail during login")
	}
}

// newTestAuthService returns a service, its querier, and the public key, so a
// test can drive the real middleware with a real version source.
func newTestAuthService(t *testing.T) (*auth.AuthService, db.Querier, ed25519.PublicKey) {
	t.Helper()
	priv, pub := testKeyPair(t)
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")
	svc := auth.NewAuthService(q, priv, pub, auth.NewRateLimiterForTest())
	return svc, q, pub
}

func seedTestUser(t *testing.T, q db.Querier, id uuid.UUID, email, password, status, role string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	if demoQ, ok := q.(*demo.InMemoryQuerier); ok {
		demoQ.SeedUserForTest(id, email, hash, status, role)
		return
	}
	_, err = q.CreateUser(context.Background(), db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		FirstName:    "Test",
		LastName:     "User",
		Role:         role,
		Status:       status,
	})
	require.NoError(t, err)
}

func setUserStatus(t *testing.T, q db.Querier, id uuid.UUID, status string) {
	t.Helper()
	err := q.UpdateUserStatus(context.Background(), db.UpdateUserStatusParams{
		ID:     pgtype.UUID{Bytes: id, Valid: true},
		Status: status,
	})
	require.NoError(t, err)
}

func countRefreshTokens(t *testing.T, q db.Querier) int {
	t.Helper()
	if demoQ, ok := q.(*demo.InMemoryQuerier); ok {
		return demoQ.RefreshTokensCountForTest()
	}
	return 0
}

func refreshParams(userID uuid.UUID, tokenHash string) db.CreateRefreshTokenParams {
	return db.CreateRefreshTokenParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
		FamilyID:  pgtype.UUID{Bytes: uuid.New(), Valid: true},
	}
}

func serveWithMiddleware(t *testing.T, svc *auth.AuthService, q db.Querier, pub ed25519.PublicKey, token string) *httptest.ResponseRecorder {
	t.Helper()
	versions, ok := q.(auth.TokenVersionVerifier)
	require.True(t, ok)
	handler := auth.AuthMiddleware(pub, versions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

type failingQuerier struct {
	db.Querier
	failCreateRefreshToken bool
}

func (f *failingQuerier) RunTx(ctx context.Context, fn func(db.Querier) error) error {
	runner, ok := f.Querier.(demo.TxRunner)
	if !ok {
		return db.ErrTransactionsUnsupported
	}
	return runner.RunTx(ctx, func(txq db.Querier) error {
		return fn(&failingQuerier{Querier: txq, failCreateRefreshToken: f.failCreateRefreshToken})
	})
}

func (f *failingQuerier) CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error) {
	if f.failCreateRefreshToken {
		return db.RefreshToken{}, errors.New("injected CreateRefreshToken failure")
	}
	return f.Querier.CreateRefreshToken(ctx, arg)
}

// TestRefreshTokenIsSingleUse: rotating consumes the presented token. The old
// 10-second grace cache handed the new pair to a second presenter.
func TestRefreshTokenIsSingleUse(t *testing.T) {
	svc, _, _ := newTestAuthService(t)
	loginRes, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)

	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.ErrorIs(t, err, auth.ErrInvalidRefreshToken, "a spent refresh token must not be redeemable twice")
}

// TestReplayRejectsTheAccessTokenIssuedBeforeIt is the §10.4 guarantee. It
// asserts the 401 through the real middleware; checking only the stored counter
// would pass even if nothing consulted it.
func TestReplayRejectsTheAccessTokenIssuedBeforeIt(t *testing.T) {
	svc, q, pub := newTestAuthService(t)
	loginRes, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	// Before the replay the token works.
	require.Equal(t, http.StatusOK, serveWithMiddleware(t, svc, q, pub, loginRes.Token).Code)

	// A legitimate rotation succeeds.
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)

	// Replaying the spent token from another client is a compromise.
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "10.0.0.9", "attacker")
	require.ErrorIs(t, err, auth.ErrInvalidRefreshToken)

	// The access token minted before all of this is now dead.
	rec := serveWithMiddleware(t, svc, q, pub, loginRes.Token)
	require.Equal(t, http.StatusUnauthorized, rec.Code,
		"without the token_version bump the stolen access token would still be accepted")
}

// TestRotationFailureDoesNotConsumeTheToken: if anything inside the transaction
// fails, the presented token must remain usable. Committing used_at first and
// failing afterwards would turn the client's next legitimate attempt into a
// theft signal.
func TestRotationFailureDoesNotConsumeTheToken(t *testing.T) {
	svc, q, _ := newTestAuthService(t)
	loginRes, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	// A querier whose CreateRefreshToken always fails: the successor insert
	// aborts, so used_at must roll back with it.
	svc.SetQuerierForTest(&failingQuerier{Querier: q, failCreateRefreshToken: true})
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.Error(t, err)

	svc.SetQuerierForTest(&failingQuerier{Querier: q, failCreateRefreshToken: false})
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err, "the injection itself must be the only difference")
}

// TestDeactivatedUserRotationDoesNotConsumeTheToken covers the ordering rule
// for the status check specifically: it must run before the commit.
func TestDeactivatedUserRotationDoesNotConsumeTheToken(t *testing.T) {
	svc, q, _ := newTestAuthService(t)
	loginRes, err := svc.Login(context.Background(), "a@example.de", "Str0ng!Pass", "", "127.0.0.1", "test")
	require.NoError(t, err)

	setUserStatus(t, q, testUserID, "DEACTIVATED")
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.Error(t, err)

	// Reactivating the user and retrying must work; the token was never consumed.
	setUserStatus(t, q, testUserID, "ACTIVE")
	_, _, err = svc.RefreshToken(context.Background(), loginRes.RefreshToken, "127.0.0.1", "test")
	require.NoError(t, err)
}

// TestDemoRefreshRollsBackLikeProduction: the demo path must have the same
// all-or-nothing semantics, or demo mode would exercise a different code path
// than production.
func TestDemoRefreshRollsBackLikeProduction(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")
	before := countRefreshTokens(t, q)

	err := q.RunTx(context.Background(), func(txq db.Querier) error {
		if _, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-a")); err != nil {
			return err
		}
		return errors.New("abort")
	})
	require.Error(t, err)
	require.Equal(t, before, countRefreshTokens(t, q), "RunTx must restore the snapshot on error")

	require.NoError(t, q.RunTx(context.Background(), func(txq db.Querier) error {
		_, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-b"))
		return err
	}))
	require.Equal(t, before+1, countRefreshTokens(t, q), "a committed RunTx must keep its writes")
}

// TestDemoRunTxDoesNotDeadlock is the guard on the mutex. InMemoryQuerier.mu is
// a sync.RWMutex, which is not reentrant.
func TestDemoRunTxDoesNotDeadlock(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	seedTestUser(t, q, testUserID, "a@example.de", "Str0ng!Pass", "ACTIVE", "VERTRIEB")

	done := make(chan error, 1)
	go func() {
		done <- q.RunTx(context.Background(), func(txq db.Querier) error {
			if _, err := txq.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true}); err != nil {
				return err
			}
			_, err := txq.CreateRefreshToken(context.Background(), refreshParams(testUserID, "hash-deadlock"))
			return err
		})
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("RunTx deadlocked: the callback must not re-enter the querier's own mutex")
	}
}

func codeFor(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	return code
}

// TestSetupTOTPStoresNothing: an unverified secret must not reach the database
// (F-03). The pre-SP8a flow wrote totp_secret_encrypted on setup.
func TestSetupTOTPStoresNothing(t *testing.T) {
	svc, q, _ := newTestAuthService(t)
	res, err := svc.SetupTOTP(context.Background(), testUserID)
	require.NoError(t, err)
	require.NotEmpty(t, res.Secret)

	user, err := q.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true})
	require.NoError(t, err)
	require.False(t, user.TotpEnabled)
	require.False(t, user.TotpSecretEncrypted.Valid, "setup must not persist a secret")

	// The first successful verification writes it and enables TOTP.
	require.NoError(t, svc.VerifyTOTP(context.Background(), testUserID, res.Secret, codeFor(t, res.Secret)))
	user, err = q.GetUserByID(context.Background(), pgtype.UUID{Bytes: testUserID, Valid: true})
	require.NoError(t, err)
	require.True(t, user.TotpEnabled)
	require.True(t, user.TotpSecretEncrypted.Valid)
}

type captureAuditLogger struct {
	actions []string
}

func (c *captureAuditLogger) Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error {
	c.actions = append(c.actions, action)
	return nil
}

func TestAuthService_SecurityEventAuditing(t *testing.T) {
	svc, q, _ := setupTestAuthService(t)
	capturer := &captureAuditLogger{}
	svc = svc.WithAuditLogger(capturer)
	ctx := context.Background()

	// 1. Login failed
	_, err := svc.Login(ctx, "admin@openlocalcrm.local", "wrong-pw", "", "127.0.0.1", "TestAgent")
	require.Error(t, err)
	require.Contains(t, capturer.actions, "LOGIN_FAILED")

	// 2. Login success
	res, err := svc.Login(ctx, "admin@openlocalcrm.local", "demo123", "", "127.0.0.1", "TestAgent")
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "LOGIN_SUCCESS")

	// 3. Logout via refresh token
	err = svc.RevokeRefreshTokenWithAudit(ctx, res.RefreshToken, "127.0.0.1", "TestAgent")
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "LOGOUT")

	// 4. AuditLogout helper
	adminUser, err := q.GetUserByEmail(ctx, "admin@openlocalcrm.local")
	require.NoError(t, err)
	adminUUID := uuid.UUID(adminUser.ID.Bytes)
	svc.AuditLogout(ctx, adminUUID, "127.0.0.1", "TestAgent")

	// 5. TOTP setup, verify, disable
	totpRes, err := svc.SetupTOTP(ctx, adminUUID)
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "TOTP_SETUP_INITIATED")

	code := codeFor(t, totpRes.Secret)
	err = svc.VerifyTOTP(ctx, adminUUID, totpRes.Secret, code)
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "TOTP_ENABLED")

	err = svc.DisableTOTP(ctx, adminUUID, "demo123")
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "TOTP_DISABLED")

	// 6. Change password
	err = svc.ChangePassword(ctx, adminUUID, "demo123", "new-strong-password-123")
	require.NoError(t, err)
	require.Contains(t, capturer.actions, "PASSWORD_CHANGED")
}
