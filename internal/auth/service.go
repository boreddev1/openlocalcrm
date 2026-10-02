package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/crypto"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrInvalidCredentials  = errors.New("ungültige E-Mail-Adresse oder Passwort")
	ErrRateLimited         = errors.New("zu viele fehlgeschlagene Versuche. Bitte warten Sie 5 Minuten")
	ErrUserNotActive       = errors.New("dieses Benutzerkonto ist nicht aktiv")
	ErrInvalidTOTPCode     = errors.New("Ungültiger Authenticator-Code") //nolint:staticcheck // user-facing German message; capitalized noun is correct
	ErrTOTPRequired        = errors.New("totp_code_required")
	ErrPasswordTooShort    = errors.New("das Passwort muss mindestens 8 Zeichen lang sein")
	ErrInvalidOldPassword  = errors.New("das aktuelle Passwort ist ungültig")
	ErrUserNotFound        = errors.New("benutzer nicht gefunden")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
)

var (
	argon2Sem       = make(chan struct{}, 4)
	dummyArgon2Hash string
)

func init() {
	var err error
	dummyArgon2Hash, err = HashPassword("OpenLocalCRM-Mitigate-User-Enumeration-Timing-F04-F05!")
	if err != nil {
		panic("auth: failed generating dummy password hash: " + err.Error())
	}
}

func acquireArgonSem(ctx context.Context) error {
	select {
	case argon2Sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseArgonSem() {
	<-argon2Sem
}

// HashPasswordLimited hashes a password while holding one of the four Argon2
// slots. Every unauthenticated caller that derives a hash must use it: Argon2id
// with m=64MiB means a handful of concurrent derivations can exceed the
// project's < 420 MB resident-memory budget on their own.
func HashPasswordLimited(ctx context.Context, password string) (string, error) {
	if err := acquireArgonSem(ctx); err != nil {
		return "", err
	}
	defer releaseArgonSem()
	return HashPassword(password)
}

const (
	AccessTokenDuration  = 30 * time.Minute
	refreshTokenDuration = 30 * 24 * time.Hour
)

type SessionRevoker interface {
	RevokeUser(userID uuid.UUID)
}

type AuditLogger interface {
	Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error
}

type TxRunner interface {
	RunTx(ctx context.Context, fn func(db.Querier) error) error
}

type AuthService struct {
	querier             db.Querier
	privKey             ed25519.PrivateKey
	pubKey              ed25519.PublicKey
	limiter             *RateLimiter
	revoker             SessionRevoker
	audit               AuditLogger
	bootstrapSecretPath string
}

func (s *AuthService) WithSessionRevoker(r SessionRevoker) *AuthService {
	s.revoker = r
	return s
}

func (s *AuthService) WithAuditLogger(a AuditLogger) *AuthService {
	s.audit = a
	return s
}

func (s *AuthService) WithBootstrapSecretPath(path string) *AuthService {
	s.bootstrapSecretPath = path
	return s
}

func (s *AuthService) SetQuerierForTest(q db.Querier) {
	s.querier = q
}

func (s *AuthService) issueAccessToken(user db.User) (string, error) {
	userID := uuid.UUID(user.ID.Bytes)
	return GenerateAccessToken(userID, user.Email, user.Role, user.TokenVersion, s.privKey, AccessTokenDuration)
}

func generateRefreshToken() (string, error) {
	return generateRandomHex(32)
}

func (s *AuthService) RevokeRefreshToken(ctx context.Context, rawRefreshToken string) error {
	return s.RevokeRefreshTokenWithAudit(ctx, rawRefreshToken, "", "")
}

func (s *AuthService) RevokeRefreshTokenWithAudit(ctx context.Context, rawRefreshToken, ip, userAgent string) error {
	if rawRefreshToken == "" {
		return nil
	}
	tokenHash := hashToken(rawRefreshToken)
	if s.audit != nil {
		if token, err := s.querier.GetRefreshTokenByHash(ctx, tokenHash); err == nil {
			_ = s.audit.Log(ctx, token.UserID, "SESSION", pgtype.UUID{}, "LOGOUT", map[string]any{
				"family_id": uuid.UUID(token.FamilyID.Bytes).String(),
			}, ip, userAgent)
		}
	}
	return s.querier.DeleteRefreshToken(ctx, tokenHash)
}

func (s *AuthService) AuditLogout(ctx context.Context, userID uuid.UUID, ip, ua string) {
	if s.audit != nil {
		_ = s.audit.Log(ctx, pgtype.UUID{Bytes: userID, Valid: true}, "SESSION", pgtype.UUID{}, "LOGOUT", nil, ip, ua)
	}
}

func (s *AuthService) RevokeAllSessions(ctx context.Context, userID pgtype.UUID) error {
	err := s.runTx(ctx, func(q db.Querier) error {
		if err := q.DeleteUserRefreshTokens(ctx, userID); err != nil {
			return err
		}
		_, err := q.IncrementUserTokenVersion(ctx, userID)
		return err
	})
	if err != nil {
		return err
	}
	if s.revoker != nil {
		s.revoker.RevokeUser(uuid.UUID(userID.Bytes))
	}
	return nil
}

func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failure: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func NewAuthService(querier db.Querier, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, limiter *RateLimiter) *AuthService {
	if limiter == nil {
		limiter = NewRateLimiter(5, 5*time.Minute, 5*time.Minute)
	}
	return &AuthService{
		querier: querier,
		privKey: privKey,
		pubKey:  pubKey,
		limiter: limiter,
	}
}

type LoginResult struct {
	Token        string  `json:"token,omitempty"`
	RefreshToken string  `json:"refresh_token,omitempty"`
	TOTPRequired bool    `json:"totp_required,omitempty"`
	User         db.User `json:"user,omitempty"`
}

func (s *AuthService) Login(ctx context.Context, email, password, totpCode, ip, userAgent string) (*LoginResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		if s.audit != nil {
			_ = s.audit.Log(ctx, pgtype.UUID{}, "AUTH", pgtype.UUID{}, "LOGIN_FAILED", map[string]any{"reason": "missing_credentials"}, ip, userAgent)
		}
		return nil, ErrInvalidCredentials
	}

	rateLimitKey := fmt.Sprintf("%s:%s", ip, email)
	if locked, remaining := s.limiter.IsLocked(rateLimitKey); locked {
		if s.audit != nil {
			_ = s.audit.Log(ctx, pgtype.UUID{}, "AUTH", pgtype.UUID{}, "LOGIN_FAILED", map[string]any{"reason": "rate_limited", "email": email}, ip, userAgent)
		}
		return nil, fmt.Errorf("%w (gesperrt für noch %s)", ErrRateLimited, remaining.Round(time.Second))
	}

	user, err := s.querier.GetUserByEmail(ctx, email)
	if err != nil {
		// Mitigation against timing attacks (Finding #33 / F-04): execute real dummy Argon2id calculation under semaphore
		if semErr := acquireArgonSem(ctx); semErr == nil {
			_ = CheckPassword(dummyArgon2Hash, password)
			releaseArgonSem()
		}
		s.limiter.RecordFailure(rateLimitKey)
		if s.audit != nil {
			_ = s.audit.Log(ctx, pgtype.UUID{}, "AUTH", pgtype.UUID{}, "LOGIN_FAILED", map[string]any{"reason": "user_not_found", "email": email}, ip, userAgent)
		}
		return nil, ErrInvalidCredentials
	}

	// Always verify password under semaphore first to eliminate user enumeration (F-04, F-05)
	if semErr := acquireArgonSem(ctx); semErr != nil {
		return nil, semErr
	}
	passwordMatches := CheckPassword(user.PasswordHash, password)
	releaseArgonSem()

	if !passwordMatches {
		s.limiter.RecordFailure(rateLimitKey)
		if s.audit != nil {
			_ = s.audit.Log(ctx, user.ID, "USER", user.ID, "LOGIN_FAILED", map[string]any{"reason": "invalid_password"}, ip, userAgent)
		}
		return nil, ErrInvalidCredentials
	}

	if user.Status != "ACTIVE" {
		s.limiter.RecordFailure(rateLimitKey)
		if s.audit != nil {
			_ = s.audit.Log(ctx, user.ID, "USER", user.ID, "LOGIN_FAILED", map[string]any{"reason": "user_not_active"}, ip, userAgent)
		}
		return nil, ErrInvalidCredentials
	}

	// 2FA Verification
	if user.TotpEnabled {
		if totpCode == "" {
			return &LoginResult{
				TOTPRequired: true,
			}, nil
		}

		encryptedSecret := user.TotpSecretEncrypted.String
		plainSecret, decErr := crypto.DecryptSecret(encryptedSecret)
		if decErr != nil {
			s.limiter.RecordFailure(rateLimitKey)
			if s.audit != nil {
				_ = s.audit.Log(ctx, user.ID, "USER", user.ID, "LOGIN_FAILED", map[string]any{"reason": "totp_decrypt_failed"}, ip, userAgent)
			}
			return nil, ErrInvalidTOTPCode
		}

		valid, usedStep := ValidateTOTPCodeWithStep(totpCode, plainSecret, user.TotpLastUsedStep)
		if !valid {
			s.limiter.RecordFailure(rateLimitKey)
			if s.audit != nil {
				_ = s.audit.Log(ctx, user.ID, "USER", user.ID, "LOGIN_FAILED", map[string]any{"reason": "invalid_totp_code"}, ip, userAgent)
			}
			return nil, ErrInvalidTOTPCode
		}

		// Persist used step to prevent replay attacks (F-02)
		if stepErr := s.querier.UpdateUserTOTPLastUsedStep(ctx, db.UpdateUserTOTPLastUsedStepParams{
			ID:               user.ID,
			TotpLastUsedStep: usedStep,
		}); stepErr != nil {
			log.Printf("[AUTH] Warning: failed updating totp_last_used_step for user %s: %v", user.Email, stepErr)
		}
	}

	// Authentication succeeded -> Reset RateLimiter
	s.limiter.Reset(rateLimitKey)

	// Update last login
	if err = s.querier.UpdateUserLastLogin(ctx, db.UpdateUserLastLoginParams{
		ID:          user.ID,
		LastLoginAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}); err != nil {
		log.Printf("[AUTH] Warning: failed updating last login for user %s: %v", user.Email, err)
	}

	// Generate Access Token (JWT with Ed25519)
	token, err := s.issueAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed generating access token: %w", err)
	}

	// Generate Refresh Token
	rawRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed generating refresh token entropy: %w", err)
	}
	tokenHash := hashToken(rawRefreshToken)
	familyID := uuid.New()

	if _, err := s.querier.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
		IpAddress: pgtype.Text{String: ip, Valid: ip != ""},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(refreshTokenDuration), Valid: true},
		FamilyID:  pgtype.UUID{Bytes: familyID, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("failed persisting refresh token: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, user.ID, "USER", user.ID, "LOGIN_SUCCESS", map[string]any{
			"email": user.Email,
		}, ip, userAgent)
	}

	return &LoginResult{
		Token:        token,
		RefreshToken: rawRefreshToken,
		User:         user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, rawRefreshToken, ip, userAgent string) (string, string, error) {
	if rawRefreshToken == "" {
		return "", "", ErrInvalidRefreshToken
	}
	hash := hashToken(rawRefreshToken)

	existing, err := s.querier.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrInvalidRefreshToken
		}
		return "", "", err
	}
	// An expired token is treated as unknown. Revoking a family because an old
	// cookie was replayed would punish the user for a stale cookie.
	if time.Now().After(existing.ExpiresAt.Time) {
		return "", "", ErrInvalidRefreshToken
	}
	if existing.UsedAt.Valid {
		s.handleReplay(ctx, existing, ip, userAgent)
		return "", "", ErrInvalidRefreshToken
	}

	var accessToken, newRefresh string
	err = s.runTx(ctx, func(q db.Querier) error {
		consumed, txErr := q.ConsumeRefreshToken(ctx, hash)
		if txErr != nil {
			// Another request consumed it between the read and here. That is a
			// race, not a theft, so roll back and let the caller retry.
			return ErrInvalidRefreshToken
		}
		// The status check runs before the commit. Checking it afterwards would
		// leave the old token consumed while the client got an error, so the
		// next attempt would look like a replay.
		user, txErr := q.GetUserByID(ctx, consumed.UserID)
		if txErr != nil {
			return txErr
		}
		if user.Status != "ACTIVE" {
			return ErrUserNotActive
		}
		if newRefresh, txErr = generateRefreshToken(); txErr != nil {
			return txErr
		}
		if _, txErr = q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID:    consumed.UserID,
			TokenHash: hashToken(newRefresh),
			UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
			IpAddress: pgtype.Text{String: ip, Valid: ip != ""},
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(refreshTokenDuration), Valid: true},
			FamilyID:  consumed.FamilyID,
		}); txErr != nil {
			return txErr
		}
		// The access token is signed before the commit for the same reason: a
		// signing failure must not leave the rotation half applied.
		accessToken, txErr = s.issueAccessToken(user)
		return txErr
	})
	if err != nil {
		return "", "", err
	}
	return accessToken, newRefresh, nil
}

func (s *AuthService) handleReplay(ctx context.Context, spent db.RefreshToken, ip, userAgent string) {
	reason := "refresh_token_reuse"
	if spent.UserAgent.String != userAgent || spent.IpAddress.String != ip {
		reason = "refresh_token_reuse_other_client"
	}

	revoked := s.runTx(ctx, func(q db.Querier) error {
		if err := q.DeleteRefreshTokenFamily(ctx, spent.FamilyID); err != nil {
			return err
		}
		// The error is not ignored: a family revocation without the version bump
		// would leave the stolen access token alive.
		_, err := q.IncrementUserTokenVersion(ctx, spent.UserID)
		return err
	}) == nil

	// The streams are closed only after a successful commit, so a failed
	// transaction cannot disconnect a user whose session was never invalidated.
	if revoked && s.revoker != nil {
		s.revoker.RevokeUser(uuid.UUID(spent.UserID.Bytes))
	}

	// The audit is unconditional: a detected replay must be visible even when
	// the revocation itself failed.
	if s.audit != nil {
		_ = s.audit.Log(ctx, spent.UserID, "AUTH", pgtype.UUID{}, "REFRESH_TOKEN_REUSE", map[string]any{
			"reason":         reason,
			"family_id":      uuid.UUID(spent.FamilyID.Bytes).String(),
			"revoked":        revoked,
			"detected_ip":    ip,
			"detected_agent": userAgent,
		}, ip, userAgent)
	}
}

func (s *AuthService) runTx(ctx context.Context, fn func(db.Querier) error) error {
	runner, ok := s.querier.(TxRunner)
	if !ok {
		return db.ErrTransactionsUnsupported
	}
	return runner.RunTx(ctx, fn)
}

type TOTPSetupResult struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
}

func (s *AuthService) SetupTOTP(ctx context.Context, userID uuid.UUID) (*TOTPSetupResult, error) {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.querier.GetUserByID(ctx, pgID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	secret, url, err := GenerateTOTPKey(user.Email)
	if err != nil {
		return nil, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, pgID, "USER", pgID, "TOTP_SETUP_INITIATED", map[string]any{
			"user_id": userID.String(),
		}, "", "")
	}

	return &TOTPSetupResult{
		Secret: secret,
		URL:    url,
	}, nil
}

func (s *AuthService) VerifyTOTP(ctx context.Context, userID uuid.UUID, secret, code string) error {
	valid, usedStep := ValidateTOTPCodeWithStep(code, secret, 0)
	if !valid {
		return ErrInvalidTOTPCode
	}

	encrypted, err := crypto.EncryptSecret(secret)
	if err != nil {
		return fmt.Errorf("failed encrypting totp secret: %w", err)
	}

	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	if err := s.querier.EnableTOTPWithSecret(ctx, db.EnableTOTPWithSecretParams{
		ID:                  pgID,
		TotpSecretEncrypted: pgtype.Text{String: encrypted, Valid: true},
	}); err != nil {
		return err
	}

	_ = s.querier.UpdateUserTOTPLastUsedStep(ctx, db.UpdateUserTOTPLastUsedStepParams{
		ID:               pgID,
		TotpLastUsedStep: usedStep,
	})

	_, _ = s.querier.IncrementUserTokenVersion(ctx, pgID)
	if s.audit != nil {
		_ = s.audit.Log(ctx, pgID, "USER", pgID, "TOTP_ENABLED", map[string]any{
			"user_id": userID.String(),
		}, "", "")
	}
	return nil
}

func (s *AuthService) DisableTOTP(ctx context.Context, userID uuid.UUID, currentPassword string) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.querier.GetUserByID(ctx, pgID)
	if err != nil {
		return ErrUserNotFound
	}

	if semErr := acquireArgonSem(ctx); semErr != nil {
		return semErr
	}
	pwMatches := CheckPassword(user.PasswordHash, currentPassword)
	releaseArgonSem()

	if !pwMatches {
		return ErrInvalidOldPassword
	}

	if err := s.querier.UpdateUserTOTP(ctx, db.UpdateUserTOTPParams{
		ID:                  pgID,
		TotpSecretEncrypted: pgtype.Text{Valid: false},
		TotpEnabled:         false,
		TotpLastUsedStep:    0,
	}); err != nil {
		return err
	}

	_, _ = s.querier.IncrementUserTokenVersion(ctx, pgID)
	if s.audit != nil {
		_ = s.audit.Log(ctx, pgID, "USER", pgID, "TOTP_DISABLED", map[string]any{
			"user_id": userID.String(),
		}, "", "")
	}
	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}

	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.querier.GetUserByID(ctx, pgID)
	if err != nil {
		return ErrUserNotFound
	}

	if semErr := acquireArgonSem(ctx); semErr != nil {
		return semErr
	}
	oldMatches := CheckPassword(user.PasswordHash, oldPassword)
	releaseArgonSem()

	if !oldMatches {
		return ErrInvalidOldPassword
	}

	if semErr := acquireArgonSem(ctx); semErr != nil {
		return semErr
	}
	hash, err := HashPassword(newPassword)
	releaseArgonSem()
	if err != nil {
		return fmt.Errorf("failed hashing password: %w", err)
	}

	if err := s.querier.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           pgID,
		PasswordHash: hash,
	}); err != nil {
		return err
	}

	_ = s.querier.SetUserMustChangePassword(ctx, db.SetUserMustChangePasswordParams{
		ID:                 pgID,
		MustChangePassword: false,
	})

	if s.bootstrapSecretPath != "" {
		_ = os.Remove(s.bootstrapSecretPath)
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, pgID, "USER", pgID, "PASSWORD_CHANGED", map[string]any{
			"user_id": userID.String(),
		}, "", "")
	}

	return nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
