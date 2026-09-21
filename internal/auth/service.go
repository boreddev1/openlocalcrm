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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrInvalidCredentials = errors.New("ungültige E-Mail-Adresse oder Passwort")
	ErrRateLimited        = errors.New("zu viele fehlgeschlagene Versuche. Bitte warten Sie 5 Minuten")
	ErrUserNotActive      = errors.New("dieses Benutzerkonto ist nicht aktiv")
	ErrInvalidTOTPCode    = errors.New("Ungültiger Authenticator-Code")
	ErrTOTPRequired       = errors.New("totp_code_required")
	ErrPasswordTooShort   = errors.New("das Passwort muss mindestens 8 Zeichen lang sein")
	ErrInvalidOldPassword = errors.New("das aktuelle Passwort ist ungültig")
	ErrUserNotFound       = errors.New("benutzer nicht gefunden")
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

type rotatedTokenEntry struct {
	accessToken  string
	refreshToken string
	rotatedAt    time.Time
}

type AuthService struct {
	querier    db.Querier
	privKey    ed25519.PrivateKey
	pubKey     ed25519.PublicKey
	limiter    *RateLimiter
	graceCache sync.Map // oldTokenHash -> rotatedTokenEntry
}

const (
	AccessTokenDuration = 30 * time.Minute
)

var revokedAccessTokens sync.Map // tokenString -> expiresAt time.Time

func RevokeToken(token string, expiresAt time.Time) {
	if token != "" {
		revokedAccessTokens.Store(token, expiresAt)
	}
}

func IsTokenRevoked(token string) bool {
	if token == "" {
		return false
	}
	exp, ok := revokedAccessTokens.Load(token)
	if !ok {
		return false
	}
	if expiresAt, ok := exp.(time.Time); ok && time.Now().After(expiresAt) {
		revokedAccessTokens.Delete(token)
		return false
	}
	return true
}

func (s *AuthService) RevokeAccessToken(token string) {
	if token == "" {
		return
	}
	claims, err := ValidateAccessToken(token, s.pubKey)
	expiresAt := time.Now().Add(AccessTokenDuration)
	if err == nil && claims != nil && claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}
	RevokeToken(token, expiresAt)
}

func (s *AuthService) IsAccessTokenRevoked(token string) bool {
	return IsTokenRevoked(token)
}

func (s *AuthService) RevokeRefreshToken(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	tokenHash := hashToken(rawRefreshToken)
	return s.querier.DeleteRefreshToken(ctx, tokenHash)
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
		return nil, ErrInvalidCredentials
	}

	rateLimitKey := fmt.Sprintf("%s:%s", ip, email)
	if locked, remaining := s.limiter.IsLocked(rateLimitKey); locked {
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
		return nil, ErrInvalidCredentials
	}

	if user.Status != "ACTIVE" {
		s.limiter.RecordFailure(rateLimitKey)
		return nil, ErrInvalidCredentials
	}

	// 2FA Verification
	if user.TotpEnabled {
		if totpCode == "" {
			return &LoginResult{
				TOTPRequired: true,
			}, nil
		}

		secret := user.TotpSecretEncrypted.String
		if !ValidateTOTPCode(totpCode, secret) {
			s.limiter.RecordFailure(rateLimitKey)
			return nil, ErrInvalidTOTPCode
		}
	}

	// Authentication succeeded -> Reset RateLimiter
	s.limiter.Reset(rateLimitKey)

	// Update last login
	if err := s.querier.UpdateUserLastLogin(ctx, db.UpdateUserLastLoginParams{
		ID:          user.ID,
		LastLoginAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}); err != nil {
		log.Printf("[AUTH] Warning: failed updating last login for user %s: %v", user.Email, err)
	}

	// Generate Access Token (JWT with Ed25519)
	userID := uuid.UUID(user.ID.Bytes)
	token, err := GenerateAccessToken(userID, user.Email, user.Role, s.privKey, AccessTokenDuration)
	if err != nil {
		return nil, fmt.Errorf("failed generating access token: %w", err)
	}

	// Generate Refresh Token
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("failed generating refresh token entropy: %w", err)
	}
	rawRefreshToken := hex.EncodeToString(rawBytes)
	tokenHash := hashToken(rawRefreshToken)

	if _, err := s.querier.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: tokenHash,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
		IpAddress: pgtype.Text{String: ip, Valid: ip != ""},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("failed persisting refresh token: %w", err)
	}

	return &LoginResult{
		Token:        token,
		RefreshToken: rawRefreshToken,
		User:         user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, rawRefreshToken, ip, userAgent string) (string, string, error) {
	if rawRefreshToken == "" {
		return "", "", errors.New("refresh token required")
	}

	tokenHash := hashToken(rawRefreshToken)

	// Check 10-second grace cache for parallel tab refresh race conditions
	if entryVal, ok := s.graceCache.Load(tokenHash); ok {
		entry := entryVal.(rotatedTokenEntry)
		if time.Since(entry.rotatedAt) < 10*time.Second {
			return entry.accessToken, entry.refreshToken, nil
		}
		s.graceCache.Delete(tokenHash)
	}

	rf, err := s.querier.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return "", "", errors.New("invalid or expired refresh token")
	}

	user, err := s.querier.GetUserByID(ctx, rf.UserID)
	if err != nil || user.Status != "ACTIVE" {
		return "", "", errors.New("user account invalid or inactive")
	}

	// Single-Use Rotation: delete old token from DB
	_ = s.querier.DeleteRefreshToken(ctx, tokenHash)

	userID := uuid.UUID(user.ID.Bytes)
	newToken, err := GenerateAccessToken(userID, user.Email, user.Role, s.privKey, AccessTokenDuration)
	if err != nil {
		return "", "", fmt.Errorf("failed generating access token: %w", err)
	}

	// Generate new refresh token
	newRawRF, err := generateRandomHex(32)
	if err != nil {
		return "", "", fmt.Errorf("failed generating refresh token entropy: %w", err)
	}
	newHash := hashToken(newRawRF)
	if _, err := s.querier.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    rf.UserID,
		TokenHash: newHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
		IpAddress: pgtype.Text{String: ip, Valid: true},
		UserAgent: pgtype.Text{String: userAgent, Valid: true},
	}); err != nil {
		return "", "", fmt.Errorf("failed persisting rotated refresh token: %w", err)
	}

	// Store in grace cache for 10 seconds
	s.graceCache.Store(tokenHash, rotatedTokenEntry{
		accessToken:  newToken,
		refreshToken: newRawRF,
		rotatedAt:    time.Now(),
	})

	return newToken, newRawRF, nil
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

	// Store secret temporarily (not enabled until verified)
	err = s.querier.UpdateUserTOTP(ctx, db.UpdateUserTOTPParams{
		ID:                  pgID,
		TotpSecretEncrypted: pgtype.Text{String: secret, Valid: true},
		TotpEnabled:         false,
	})
	if err != nil {
		return nil, err
	}

	return &TOTPSetupResult{
		Secret: secret,
		URL:    url,
	}, nil
}

func (s *AuthService) VerifyTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	pgID := pgtype.UUID{Bytes: userID, Valid: true}
	user, err := s.querier.GetUserByID(ctx, pgID)
	if err != nil {
		return ErrUserNotFound
	}

	secret := user.TotpSecretEncrypted.String
	if secret == "" {
		return errors.New("no TOTP setup found in progress")
	}

	if !ValidateTOTPCode(code, secret) {
		return ErrInvalidTOTPCode
	}

	return s.querier.UpdateUserTOTP(ctx, db.UpdateUserTOTPParams{
		ID:                  pgID,
		TotpSecretEncrypted: pgtype.Text{String: secret, Valid: true},
		TotpEnabled:         true,
	})
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

	return s.querier.UpdateUserTOTP(ctx, db.UpdateUserTOTPParams{
		ID:                  pgID,
		TotpSecretEncrypted: pgtype.Text{Valid: false},
		TotpEnabled:         false,
	})
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

	return s.querier.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           pgID,
		PasswordHash: hash,
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
