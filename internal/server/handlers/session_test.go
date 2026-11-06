package handlers_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestSessionHandlerLogoutAll(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	q := demo.NewEmptyInMemoryQuerier()
	hash, err := auth.HashPassword("Str0ng!Pass")
	require.NoError(t, err)

	_, err = q.CreateUser(context.Background(), db.CreateUserParams{
		Email:        "user@example.com",
		PasswordHash: hash,
		FirstName:    "Test",
		LastName:     "User",
		Role:         "VERTRIEB",
	})
	require.NoError(t, err)

	u, err := q.GetUserByEmail(context.Background(), "user@example.com")
	require.NoError(t, err)

	authSvc := auth.NewAuthService(q, priv, pub, auth.NewRateLimiterForTest())
	hub := sse.NewHub()
	h := handlers.NewSessionHandler(authSvc, hub)

	// 1. Unauthorized when no claims
	reqNoAuth := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout-all", nil)
	recNoAuth := httptest.NewRecorder()
	h.LogoutAll(recNoAuth, reqNoAuth)
	require.Equal(t, http.StatusUnauthorized, recNoAuth.Code)

	// 2. Successful logout-all when authenticated
	claims := &auth.AccessClaims{
		UserID:       u.ID.Bytes,
		Email:        u.Email,
		Role:         u.Role,
		TokenVersion: &u.TokenVersion,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, claims))
	rec := httptest.NewRecorder()

	h.LogoutAll(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify cookies cleared
	cookies := rec.Result().Cookies()
	var accessCleared, refreshCleared bool
	for _, c := range cookies {
		if c.Name == "access_token" && c.MaxAge == -1 {
			accessCleared = true
		}
		if c.Name == "refresh_token" && c.MaxAge == -1 {
			refreshCleared = true
		}
	}
	require.True(t, accessCleared, "access_token cookie must be cleared")
	require.True(t, refreshCleared, "refresh_token cookie must be cleared")

	// Verify token version bumped in database
	ver, err := q.CurrentTokenVersion(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, u.TokenVersion+1, ver, "token_version must be incremented")
}
