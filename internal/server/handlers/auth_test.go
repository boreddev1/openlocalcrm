package handlers_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestAuthHandlerLogout(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	h := handlers.NewAuthHandler(nil, privKey, pubKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var foundAuthCookie bool
	for _, c := range cookies {
		if c.Name == "access_token" && c.MaxAge == -1 {
			foundAuthCookie = true
		}
	}
	if !foundAuthCookie {
		t.Fatalf("expected cleared access_token cookie")
	}
}

func TestAuthHandlerLoginValidation(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	h := handlers.NewAuthHandler(nil, privKey, pubKey)

	// Missing email & password
	payload := []byte(`{"email":"","password":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func newTestAuthHandler(t *testing.T) (*handlers.AuthHandler, db.Querier) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	q := demo.NewEmptyInMemoryQuerier()
	hash, err := auth.HashPassword("Str0ng!Pass")
	require.NoError(t, err)
	_, err = q.CreateUser(context.Background(), db.CreateUserParams{
		Email:        "a@example.de",
		PasswordHash: hash,
		FirstName:    "A",
		LastName:     "B",
		Role:         "ADMIN",
		Status:       "ACTIVE",
	})
	require.NoError(t, err)
	svc := auth.NewAuthService(q, priv, pub, auth.NewRateLimiterForTest())
	return handlers.NewAuthHandlerWithService(svc), q
}

func performJSON(t *testing.T, h *handlers.AuthHandler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	switch path {
	case "/api/v1/auth/login":
		h.Login(rec, req)
	case "/api/v1/auth/refresh":
		h.Refresh(rec, req)
	}
	return rec
}

// TestLoginResponseCarriesNoToken: the cookie is the only channel. A raw token
// in a response body is readable by anything that can see the body.
func TestLoginResponseCarriesNoToken(t *testing.T) {
	h, _ := newTestAuthHandler(t)
	rec := performJSON(t, h, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "a@example.de", "password": "Str0ng!Pass",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, rec.Result().Cookies())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotContains(t, body, "token")
	require.NotContains(t, body, "refresh_token")
}

// TestRefreshRejectsABodyToken: once the response carries no token, accepting a
// body token would strand a non-browser client with no way to continue.
func TestRefreshRejectsABodyToken(t *testing.T) {
	h, _ := newTestAuthHandler(t)
	rec := performJSON(t, h, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": "whatever",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
