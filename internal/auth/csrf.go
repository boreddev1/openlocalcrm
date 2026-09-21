package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const (
	CSRFCookieName = "csrf_token"
	CSRFHeaderName = "X-CSRF-Token"
)

// GenerateCSRFToken generates a cryptographically secure 32-byte hex token
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetCSRFCookie sets or refreshes the readable CSRF cookie for the client SPA
func SetCSRFCookie(w http.ResponseWriter, secure bool) string {
	token, err := GenerateCSRFToken()
	if err != nil {
		token = hex.EncodeToString([]byte(time.Now().String()))
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

	return token
}

// ClearCSRFCookie invalidates the CSRF cookie on logout
func ClearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: false,
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

		// 2. Requests using Authorization: Bearer <token> (such as connectors, webhooks, or API clients)
		// are not ambient-credentialed and are therefore immune to browser CSRF
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		// 3. For cookie-authenticated mutating requests, check Double Submit Cookie
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

		next.ServeHTTP(w, r)
	})
}
