package auth

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"strings"
)

type contextKey string

const (
	UserContextKey contextKey = "user_claims"
)

// AuthMiddleware creates an HTTP middleware that validates Ed25519 JWT tokens from Authorization header or cookie
func AuthMiddleware(pubKey ed25519.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// 1. Check Authorization Bearer header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// 2. Fallback to HttpOnly cookie
			if tokenStr == "" {
				if cookie, err := r.Cookie("access_token"); err == nil {
					tokenStr = cookie.Value
				}
			}

			if tokenStr == "" {
				http.Error(w, `{"error":"unauthorized","message":"missing authentication token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ValidateAccessToken(tokenStr, pubKey)
			if err != nil {
				http.Error(w, `{"error":"unauthorized","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole enforces specific roles for authenticated users
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(UserContextKey).(*AccessClaims)
			if !ok || claims == nil || claims.Role != role {
				http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
