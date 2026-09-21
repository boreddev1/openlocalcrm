package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestAuthMiddleware_BearerSoleAuthPathSetsAuthViaBearer(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	var capturedAuthViaBearer bool
	handler := auth.AuthMiddleware(pubKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthViaBearer = auth.IsAuthViaBearer(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, "user@example.com", "ADMIN", privKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !capturedAuthViaBearer {
		t.Fatalf("expected auth_via_bearer to be true for pure bearer request")
	}
}

func TestAuthMiddleware_CookieAuthDoesNotSetAuthViaBearer(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	var capturedAuthViaBearer bool
	handler := auth.AuthMiddleware(pubKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthViaBearer = auth.IsAuthViaBearer(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, "user@example.com", "ADMIN", privKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedAuthViaBearer {
		t.Fatalf("expected auth_via_bearer to be false for cookie-authenticated request")
	}
}

func TestAuthMiddleware_CookiePlusInvalidBearerFails(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	handler := auth.AuthMiddleware(pubKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, "user@example.com", "ADMIN", privKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// Request with valid cookie but invalid Bearer header must fail 401 (no fallback to cookie)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	req.Header.Set("Authorization", "Bearer invalid-garbage-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid Bearer header, got %d", rec.Code)
	}
}

func TestAuthMiddleware_CookiePlusValidBearerDoesNotSetAuthViaBearer(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	var capturedAuthViaBearer bool
	handler := auth.AuthMiddleware(pubKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthViaBearer = auth.IsAuthViaBearer(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	token, err := auth.GenerateAccessToken(userID, "user@example.com", "ADMIN", privKey, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// Request with session cookie present must not set auth_via_bearer
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedAuthViaBearer {
		t.Fatalf("expected auth_via_bearer to be false when session cookie is present")
	}
}
