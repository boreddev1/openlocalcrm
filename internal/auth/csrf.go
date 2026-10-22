package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	CSRFCookieName = "csrf_token"
	CSRFHeaderName = "X-CSRF-Token"
	// CSRFSecretLength is the minimum HMAC key material. The secret used to be an
	// in-memory random value, which made 32 bytes a convention rather than a rule;
	// now that it can be installed from disk the floor has to be stated.
	CSRFSecretLength = 32
	// csrfHKDFInfo domain-binds the derived secret: the same signing key must
	// never produce the JWT seed, a session key and the CSRF HMAC key.
	csrfHKDFInfo = "openlocalcrm:csrf-secret:v1"
)

// csrfSecretHolder guards the HMAC key. Reads and writes were previously
// unsynchronised (SetCSRFSecret wrote while getCSRFSecret read), so "install it
// once at startup" was only safe by luck of ordering.
type csrfSecretHolder struct {
	mu            sync.RWMutex
	secret        []byte
	installed     bool
	inUse         bool
	ephemeralNote sync.Once
}

var csrfKeys = &csrfSecretHolder{}

// get returns the installed secret, or lazily creates an ephemeral per-process
// one and says so out loud: an uninstalled secret is a misconfiguration, not a
// silent security downgrade.
func (h *csrfSecretHolder) get() []byte {
	h.mu.RLock()
	secret := h.secret
	h.mu.RUnlock()
	if secret != nil {
		return secret
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.secret == nil {
		b := make([]byte, CSRFSecretLength)
		if _, err := rand.Read(b); err != nil {
			panic("failed to initialize CSRF secret: " + err.Error())
		}
		h.secret = b
		h.inUse = true
		h.ephemeralNote.Do(func() {
			log.Print("[WARN] CSRF secret not installed - using an ephemeral per-process key; every restart invalidates outstanding CSRF tokens")
		})
	}
	return h.secret
}

// install stores the secret. trim is deliberately a parameter instead of a
// behavior: whitespace trimming is for an operator's file (echo leaves a trailing
// newline), while key material from a KDF is binary and may legitimately contain
// 0x09-0x0D or 0x20 - trimming that would silently shorten a valid 32-byte secret
// to 31 and be rejected, or worse, differ between processes.
//
// First use wins: once a secret is installed or the ephemeral fallback has been
// generated, a later install is refused. Swapping the HMAC key while tokens are
// live would invalidate them silently, which is the failure this whole file exists
// to prevent.
func (h *csrfSecretHolder) install(s []byte, trim bool) error {
	material := s
	if trim {
		material = bytes.TrimSpace(s)
	}
	if len(material) < CSRFSecretLength {
		return fmt.Errorf("csrf secret too short: %d usable bytes, need at least %d", len(material), CSRFSecretLength)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.installed {
		return errors.New("csrf secret already installed; refusing to overwrite it")
	}
	if h.inUse {
		return errors.New("csrf secret already in use (ephemeral key was generated); refusing to swap it")
	}
	secret := make([]byte, len(material))
	copy(secret, material)
	h.secret = secret
	h.installed = true
	return nil
}

func getCSRFSecret() []byte { return csrfKeys.get() }

// SetCSRFSecret installs the CSRF HMAC key exactly once, before serving traffic.
// Surrounding whitespace is trimmed here: this is the operator-facing path.
func SetCSRFSecret(s []byte) error { return csrfKeys.install(s, true) }

// DeriveCSRFSecret derives the CSRF HMAC key from the server's Ed25519 signing
// key with HKDF-SHA256 (RFC 5869), using only the 32-byte private seed. The
// signing key is already persisted and already rotated, so persistence and
// rotation of the CSRF secret are not a second, separately forgettable task.
func DeriveCSRFSecret(signingKey ed25519.PrivateKey) ([]byte, error) {
	if len(signingKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("cannot derive csrf secret: signing key is %d bytes, want %d", len(signingKey), ed25519.PrivateKeySize)
	}
	seed := signingKey.Seed()
	if bytes.Trim(seed, "\x00") == nil {
		// HKDF would happily derive from an all-zero seed, which would hand every
		// installation the same CSRF key - a zero key is a broken file, not input.
		return nil, errors.New("cannot derive csrf secret: signing key seed is empty")
	}
	secret, err := hkdf.Key(sha256.New, seed, nil, csrfHKDFInfo, CSRFSecretLength)
	if err != nil {
		return nil, fmt.Errorf("cannot derive csrf secret: %w", err)
	}
	return secret, nil
}

// InstallCSRFSecretFromSigningKey derives and installs in one step.
func InstallCSRFSecretFromSigningKey(signingKey ed25519.PrivateKey) error {
	secret, err := DeriveCSRFSecret(signingKey)
	if err != nil {
		return err
	}
	// Binary key material must reach the holder untouched.
	return csrfKeys.install(secret, false)
}

// GenerateBoundCSRFToken generates an HMAC-bound 32-byte hex nonce with HMAC hex
func GenerateBoundCSRFToken(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonceHex := hex.EncodeToString(b)
	subject := "anonymous"
	if userID != "" {
		subject = userID
	}
	mac := hmac.New(sha256.New, getCSRFSecret())
	mac.Write([]byte(nonceHex + ":" + subject))
	hmacHex := hex.EncodeToString(mac.Sum(nil))
	return nonceHex + "." + hmacHex, nil
}

// GenerateCSRFToken generates a cryptographically secure CSRF token
func GenerateCSRFToken() (string, error) {
	return GenerateBoundCSRFToken("")
}

// ValidateCSRFToken verifies that the token was signed with the server CSRF secret
// and matches either the authenticated user ID or anonymous session
func ValidateCSRFToken(token string, userID string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	nonceHex, hmacHex := parts[0], parts[1]
	receivedHMAC, err := hex.DecodeString(hmacHex)
	if err != nil {
		return false
	}

	check := func(subject string) bool {
		mac := hmac.New(sha256.New, getCSRFSecret())
		mac.Write([]byte(nonceHex + ":" + subject))
		expected := mac.Sum(nil)
		return subtle.ConstantTimeCompare(receivedHMAC, expected) == 1
	}

	if userID != "" && check(userID) {
		return true
	}
	return check("anonymous")
}

// SetCSRFCookie sets or refreshes the readable CSRF cookie for the client SPA
func SetCSRFCookie(w http.ResponseWriter, secure bool, userID ...string) (string, error) {
	uID := ""
	if len(userID) > 0 {
		uID = userID[0]
	}
	token, err := GenerateBoundCSRFToken(uID)
	if err != nil {
		http.Error(w, `{"error":"internal_error","message":"failed to generate CSRF token"}`, http.StatusInternalServerError)
		return "", err
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly is intentionally false (double-submit CSRF cookie must be readable by JS); Secure is set dynamically
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: false, // Must be readable by JavaScript client to send in header
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	return token, nil
}

// ClearCSRFCookie invalidates the CSRF cookie on logout
func ClearCSRFCookie(w http.ResponseWriter, secure ...bool) {
	isSecure := true
	if len(secure) > 0 {
		isSecure = secure[0]
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly is intentionally false (double-submit CSRF cookie must be readable by JS); Secure is set dynamically
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: false,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// CSRFProtectionMiddleware enforces double-submit cookie protection on mutating HTTP methods
func CSRFProtectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Safe methods are allowed without CSRF check
		method := r.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace {
			next.ServeHTTP(w, r)
			return
		}

		// 2. Login endpoint establishes the session and issues the CSRF cookie, so it does not require an existing CSRF cookie
		if r.URL.Path == "/api/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		// 3. Requests authenticated via verified Bearer token are not ambient-credentialed and are therefore immune to browser CSRF
		if IsAuthViaBearer(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		// 4. For cookie-authenticated mutating requests, check Double Submit Cookie
		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"csrf_token_missing","message":"CSRF-Cookie fehlt"}`, http.StatusForbidden)
			return
		}

		headerToken := r.Header.Get(CSRFHeaderName)
		if headerToken == "" {
			http.Error(w, `{"error":"csrf_token_missing","message":"X-CSRF-Token Header fehlt"}`, http.StatusForbidden)
			return
		}

		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
			http.Error(w, `{"error":"csrf_token_mismatch","message":"Ungültiges CSRF-Token"}`, http.StatusForbidden)
			return
		}

		var userIDStr string
		if claims := GetUserClaims(r.Context()); claims != nil {
			userIDStr = claims.UserID.String()
		}
		if !ValidateCSRFToken(headerToken, userIDStr) {
			http.Error(w, `{"error":"csrf_token_invalid","message":"Ungültiges CSRF-Token"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRFDoubleSubmitMiddleware protects /auth/refresh and /auth/logout. Those routes are
// authenticated by the HttpOnly SameSite=Strict refresh cookie, which browsers never send
// cross-site, and they run without an access token (it expires after 30 minutes and its
// cookie is then dropped). The user-bound HMAC check of CSRFProtectionMiddleware therefore
// cannot apply; the cookie/header equality check remains as defense in depth.
func CSRFDoubleSubmitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"csrf_token_missing","message":"CSRF-Cookie fehlt"}`, http.StatusForbidden)
			return
		}
		header := r.Header.Get(CSRFHeaderName)
		if header == "" {
			http.Error(w, `{"error":"csrf_token_missing","message":"X-CSRF-Token Header fehlt"}`, http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			http.Error(w, `{"error":"csrf_token_mismatch","message":"Ungültiges CSRF-Token"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
