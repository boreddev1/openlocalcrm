package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	CSRFCookieName = "csrf_token"
	CSRFHeaderName = "X-CSRF-Token"
)

var (
	csrfSecretOnce sync.Once
	csrfSecret     []byte
)

func getCSRFSecret() []byte {
	csrfSecretOnce.Do(func() {
		csrfSecret = make([]byte, 32)
		if _, err := rand.Read(csrfSecret); err != nil {
			panic("failed to initialize CSRF secret: " + err.Error())
		}
	})
	return csrfSecret
}

// SetCSRFSecret allows explicitly configuring the CSRF secret (e.g. for testing)
func SetCSRFSecret(s []byte) {
	csrfSecret = s
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

	http.SetCookie(w, &http.Cookie{
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

	http.SetCookie(w, &http.Cookie{
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
// cannot apply; the cookie/header equality check remains as defence in depth.
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
