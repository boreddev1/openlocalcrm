package server_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestHealthEndpoint(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	r := server.NewRouter(server.Config{
		SSEHub:  sse.NewHub(),
		PubKey:  pubKey,
		PrivKey: privKey,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed decoding health response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Fatalf("expected healthy status, got %v", resp["status"])
	}
}

func TestAuthenticatedRoute(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	r := server.NewRouter(server.Config{
		SSEHub:  sse.NewHub(),
		PubKey:  pubKey,
		PrivKey: privKey,
	})

	// 1. Unauthenticated request should fail
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	unauthRec := httptest.NewRecorder()
	r.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", unauthRec.Code)
	}

	// 2. Authenticated request with Bearer token should succeed
	userID := uuid.New()
	token, _ := auth.GenerateAccessToken(userID, "admin@openlocalcrm.local", "ADMIN", privKey, 15*time.Minute)
	authReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	authReq.Header.Set("Authorization", "Bearer "+token)
	authRec := httptest.NewRecorder()
	r.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid token, got %d", authRec.Code)
	}
}
