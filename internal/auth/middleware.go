package auth

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"strings"
)

type contextKey string

const (
	UserContextKey          contextKey = "user_claims"
	AuthViaBearerContextKey contextKey = "auth_via_bearer"
)

// WithAuthViaBearer marks the context as having authenticated via Bearer token
func WithAuthViaBearer(ctx context.Context) context.Context {
	return context.WithValue(ctx, AuthViaBearerContextKey, true)
}

// IsAuthViaBearer checks if the request was authenticated via Bearer token
func IsAuthViaBearer(ctx context.Context) bool {
	v, ok := ctx.Value(AuthViaBearerContextKey).(bool)
	return ok && v
}

// AuthMiddleware creates an HTTP middleware that validates Ed25519 JWT tokens from Authorization header or cookie
func AuthMiddleware(pubKey ed25519.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string
			var fromBearer bool

			authHeader := r.Header.Get("Authorization")
			hasBearer := strings.HasPrefix(authHeader, "Bearer ")
			cookie, cookieErr := r.Cookie("access_token")
			hasCookie := cookieErr == nil && cookie != nil && cookie.Value != ""

			// 1. Check Authorization Bearer header
			if hasBearer {
				candidate := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
				if candidate == "" {
					http.Error(w, `{"error":"unauthorized","message":"missing authentication token"}`, http.StatusUnauthorized)
					return
				}
				tokenStr = candidate
				fromBearer = true
			} else if hasCookie {
				// 2. Fallback to HttpOnly cookie ONLY when no Authorization header was provided
				tokenStr = cookie.Value
			}

			if tokenStr == "" {
				http.Error(w, `{"error":"unauthorized","message":"missing authentication token"}`, http.StatusUnauthorized)
				return
			}

			if IsTokenRevoked(tokenStr) {
				http.Error(w, `{"error":"unauthorized","message":"token has been revoked"}`, http.StatusUnauthorized)
				return
			}

			claims, err := ValidateAccessToken(tokenStr, pubKey)
			if err != nil {
				http.Error(w, `{"error":"unauthorized","message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			// Only set auth_via_bearer if bearer validation was the sole auth path and no session cookie was present
			if fromBearer && !hasCookie {
				ctx = WithAuthViaBearer(ctx)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole enforces specific roles for authenticated users
func RequireRole(role string) func(http.Handler) http.Handler {
	return RequireAnyRole(role)
}

// RequireAnyRole enforces that the authenticated user has at least one of the specified roles
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(UserContextKey).(*AccessClaims)
			if !ok || claims == nil {
				http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			for _, role := range roles {
				if claims.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"forbidden","message":"insufficient permissions"}`, http.StatusForbidden)
		})
	}
}
