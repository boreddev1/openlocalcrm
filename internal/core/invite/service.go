// Package invite owns the one-time token invite flow: issuance, read-only
// validation and atomic acceptance.
//
// The security contract of this package is narrow and deliberate:
//
//   - A raw token is 32 bytes from crypto/rand, encoded with
//     base64.RawURLEncoding, and returned exactly once, to the issuing admin.
//   - Only the lowercase SHA-256 hex digest is persisted (user_invites.token_hash).
//   - A raw token is never logged, never audited and never serialized. The
//     public views below contain no field that could carry one.
//   - GET validation never mutates. Only AcceptUserInvite consumes a token, and
//     it does so through one atomic statement that also activates the account.
//
// Nothing in this package claims an email was sent. It cannot deliver mail, so
// the API hands the admin a link to copy.
package invite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// TokenTTL is how long an issued invite stays usable. The approved design
// fixes this at 72 hours.
const TokenTTL = 72 * time.Hour

// TokenBytes is the entropy of a raw invite token.
const TokenBytes = 32

// MinPasswordLength is the same floor the rest of the auth surface applies.
const MinPasswordLength = 8

// InvitePath is the SPA route an invite link points at. It is not an API path.
const InvitePath = "/accept-invite"

var (
	// ErrInviteNotUsable is the single stable error for every unusable invite:
	// unknown, consumed, revoked or expired. One error for four states means an
	// attacker learns nothing from the difference, and a caller can branch once.
	ErrInviteNotUsable = errors.New("invite is not usable")
	// ErrWeakPassword refuses a password that does not meet the auth policy. The
	// invite is left untouched so the invitee can try again.
	ErrWeakPassword = errors.New("password does not meet the minimum policy")
	// ErrInvalidRequest refuses a malformed issuance request before anything is
	// written.
	ErrInvalidRequest = errors.New("invalid invite request")
	// ErrNoGateway signals a service built without a database.
	ErrNoGateway = errors.New("invite service has no database")
)

// AuditLogger is the audit seam. It matches internal/core/backup.AuditLogger so
// the router can pass the same *audit.Service to both.
type AuditLogger interface {
	Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error
}

// RequestMeta is the real request identity recorded on every invite audit event.
// It travels in the context so the public method signatures stay free of
// plumbing arguments the service would only forward.
type RequestMeta struct {
	IPAddress string
	UserAgent string
}

type requestMetaKey struct{}

// WithRequestMeta attaches the request identity for the audit trail. The HTTP
// layer is the only place that can know it.
func WithRequestMeta(ctx context.Context, ipAddress, userAgent string) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, RequestMeta{IPAddress: ipAddress, UserAgent: userAgent})
}

func metaFrom(ctx context.Context) RequestMeta {
	meta, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta
}

// CreateUserInput is the admin-supplied invite request.
type CreateUserInput struct {
	Email     string
	FirstName string
	LastName  string
	Role      string
}

// InviteView is the safe projection of a validated invite. It deliberately has
// no token, hash or lifecycle column: db.UserInvite carries token_hash and must
// never be serialized directly.
type InviteView struct {
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IssuedInvite is the one-time result of issuance. RawToken and URL exist only
// in this struct and in the admin's HTTP response; they are never persisted,
// logged or audited.
type IssuedInvite struct {
	Invite    db.UserInvite
	RawToken  string
	URL       string
	ExpiresAt time.Time
}

// Service issues, validates and accepts invites against a db.Querier.
type Service struct {
	queries db.Querier
	audit   AuditLogger
	// now is a seam so the 72 hour window is testable without sleeping.
	now func() time.Time
}

// NewService builds the service. audit may be nil: audit is bookkeeping and
// must never turn a real invite into a fake failure, nor hide one.
func NewService(queries db.Querier, audit AuditLogger) *Service {
	return &Service{queries: queries, audit: audit, now: func() time.Time { return time.Now().UTC() }}
}

// NewRawToken returns a fresh URL-safe 32 byte token and its SHA-256 hex digest.
// Only the digest is meant to be persisted.
func NewRawToken() (raw string, hash string, err error) {
	buf := make([]byte, TokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

// HashToken returns the persisted form of a raw token. It is the only way a
// caller may turn a token from the URL into a lookup key.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// BuildInviteURL assembles the copyable link from an origin and the raw token.
//
// base is empty when no usable origin is known, in which case the link is the
// bare InvitePath. A base that already carries a scheme is used verbatim; one
// that does not is assumed to be https. Callers are responsible for sanitizing
// an untrusted host before it reaches this function — see SanitizeHost — because
// the result is a link an administrator hands to a person.
func BuildInviteURL(base, rawToken string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" && !strings.Contains(base, "://") {
		base = "https://" + base
	}
	return base + InvitePath + "?token=" + url.QueryEscape(rawToken)
}

// SanitizeHost accepts a host only if it is unambiguously one.
//
// The Host header is attacker-controlled, and the sanitized value ends up in a
// link that is displayed, copied and handed over. A value carrying a scheme, a
// path, a credential, a control character or any whitespace is refused outright
// rather than cleaned up, because guessing at a repaired version of
// attacker-influenced input is how a bypass gets written. An empty result means
// "no usable host", not "an empty host".
//
// Nothing is trimmed: a host with leading or trailing whitespace is refused
// rather than repaired. Trimming would make "crm.example.de\x0b" pass by
// accident and would contradict the refusal rule above.
func SanitizeHost(host string) string {
	if host == "" {
		return ""
	}
	if strings.ContainsAny(host, " \t\r\n\v\f/\\?#@") || strings.Contains(host, "://") {
		return ""
	}
	for _, r := range host {
		if r < 0x21 || r > 0x7e {
			return ""
		}
	}
	return host
}

var validRoles = map[string]struct{}{
	"ADMIN":      {},
	"BENUTZER":   {},
	"VERTRIEB":   {},
	"BACKOFFICE": {},
}

// Issue creates the INVITED user and its invite in one statement, and returns
// the raw token exactly once. The caller must show the returned link to the
// admin; the token is unrecoverable afterwards.
//
// baseURL is the browser-facing origin for the link. It must already be
// sanitized (SanitizeHost) when it is derived from the request, and may be empty,
// in which case the link is the bare InvitePath.
func (s *Service) Issue(ctx context.Context, actorID pgtype.UUID, req CreateUserInput, baseURL string) (IssuedInvite, error) {
	if s == nil || s.queries == nil {
		return IssuedInvite{}, ErrNoGateway
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	role := strings.ToUpper(strings.TrimSpace(req.Role))

	if email == "" || firstName == "" || lastName == "" || !validEmail(email) {
		return IssuedInvite{}, ErrInvalidRequest
	}
	if _, ok := validRoles[role]; !ok {
		return IssuedInvite{}, ErrInvalidRequest
	}

	rawToken, tokenHash, err := NewRawToken()
	if err != nil {
		return IssuedInvite{}, err
	}
	// The account gets a real Argon2id hash of a discarded random password. The
	// account is INVITED, so it cannot authenticate; AcceptUserInvite replaces
	// the hash with the password the invitee actually chose.
	discarded, err := randomPasswordHash(ctx)
	if err != nil {
		return IssuedInvite{}, err
	}

	expiresAt := s.now().Add(TokenTTL)
	invite, err := s.queries.CreateInvitedUserWithInvite(ctx, db.CreateInvitedUserWithInviteParams{
		Email:        email,
		PasswordHash: discarded,
		FirstName:    firstName,
		LastName:     lastName,
		Role:         role,
		TokenHash:    tokenHash,
		CreatedBy:    actorID,
		ExpiresAt:    pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return IssuedInvite{}, err
	}

	// The audit event names the invite and the invited account. It never carries
	// the raw token or its hash.
	s.log(ctx, actorID, "USER_INVITE_CREATED", invite.ID, map[string]any{
		"invite_id":  invite.ID,
		"user_id":    invite.UserID,
		"expires_at": expiresAt,
	})

	return IssuedInvite{
		Invite:    invite,
		RawToken:  rawToken,
		URL:       BuildInviteURL(baseURL, rawToken),
		ExpiresAt: expiresAt,
	}, nil
}

// Validate resolves a raw token to safe metadata. It is strictly read-only: no
// column is written, so opening the link twice is harmless and the account stays
// INVITED.
func (s *Service) Validate(ctx context.Context, rawToken string) (InviteView, error) {
	if s == nil || s.queries == nil {
		return InviteView{}, ErrNoGateway
	}
	invite, user, err := s.resolveActiveInvite(ctx, rawToken)
	if err != nil {
		s.log(ctx, pgtype.UUID{}, "USER_INVITE_REJECTED", pgtype.UUID{}, map[string]any{
			"operation": "validate",
		})
		return InviteView{}, err
	}
	_ = invite
	return InviteView{
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		ExpiresAt: invite.ExpiresAt.Time.UTC(),
	}, nil
}

// Accept consumes the token and activates the account in one atomic statement.
// An unknown, consumed, revoked or expired token changes nothing.
func (s *Service) Accept(ctx context.Context, rawToken, password string) (db.User, error) {
	if s == nil || s.queries == nil {
		return db.User{}, ErrNoGateway
	}

	// The password is checked before the token is touched, so a weak password
	// never consumes an invite the invitee still needs.
	if len([]rune(password)) < MinPasswordLength {
		s.log(ctx, pgtype.UUID{}, "USER_INVITE_REJECTED", pgtype.UUID{}, map[string]any{
			"operation": "accept", "reason": "weak_password",
		})
		return db.User{}, ErrWeakPassword
	}

	// Argon2id runs under the auth package's four-slot semaphore, so a burst of
	// public acceptance requests cannot blow the resident-memory budget.
	passwordHash, err := auth.HashPasswordLimited(ctx, password)
	if err != nil {
		return db.User{}, err
	}

	// One statement locks the active invite, consumes it and activates the user.
	// Zero rows means the token was not usable; nothing was written.
	user, err := s.queries.AcceptUserInvite(ctx, db.AcceptUserInviteParams{
		TokenHash:    HashToken(rawToken),
		PasswordHash: passwordHash,
	})
	if err != nil {
		s.log(ctx, pgtype.UUID{}, "USER_INVITE_REJECTED", pgtype.UUID{}, map[string]any{
			"operation": "accept", "reason": "invite_not_usable",
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, ErrInviteNotUsable
		}
		return db.User{}, err
	}

	s.log(ctx, user.ID, "USER_INVITE_ACCEPTED", user.ID, map[string]any{
		"user_id": user.ID,
		"status":  user.Status,
	})
	return user, nil
}

// resolveActiveInvite looks the token up and applies the same active predicates
// the SQL lock uses. Every failure collapses into ErrInviteNotUsable.
func (s *Service) resolveActiveInvite(ctx context.Context, rawToken string) (db.UserInvite, db.User, error) {
	if strings.TrimSpace(rawToken) == "" {
		return db.UserInvite{}, db.User{}, ErrInviteNotUsable
	}
	invite, err := s.queries.GetUserInviteByTokenHash(ctx, HashToken(rawToken))
	if err != nil {
		return db.UserInvite{}, db.User{}, ErrInviteNotUsable
	}
	now := s.now()
	switch {
	case invite.ConsumedAt.Valid, invite.RevokedAt.Valid:
		return db.UserInvite{}, db.User{}, ErrInviteNotUsable
	case !invite.ExpiresAt.Valid || !invite.ExpiresAt.Time.After(now):
		return db.UserInvite{}, db.User{}, ErrInviteNotUsable
	}
	user, err := s.queries.GetUserByID(ctx, invite.UserID)
	if err != nil {
		return db.UserInvite{}, db.User{}, ErrInviteNotUsable
	}
	return invite, user, nil
}

func (s *Service) log(ctx context.Context, userID pgtype.UUID, action string, entityID pgtype.UUID, changes any) {
	if s.audit == nil {
		return
	}
	meta := metaFrom(ctx)
	_ = s.audit.Log(ctx, userID, "USER", entityID, action, changes, meta.IPAddress, meta.UserAgent)
}

func randomPasswordHash(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// The plaintext is discarded immediately and never returned, logged or
	// stored anywhere but the users.password_hash column as a hash.
	return auth.HashPasswordLimited(ctx, base64.RawURLEncoding.EncodeToString(buf))
}

func validEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	if strings.Count(email, "@") != 1 {
		return false
	}
	if strings.ContainsAny(email, " \t\r\n\"'<>()[]{}") {
		return false
	}
	domain := email[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return !strings.Contains(email, "..")
}
