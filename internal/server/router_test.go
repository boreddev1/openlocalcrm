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
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
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

func TestRBAC_AdminEndpointsProtection(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	userID := uuid.New()
	benutzerToken, _ := auth.GenerateAccessToken(userID, "user@openlocalcrm.local", "BENUTZER", privKey, 15*time.Minute)
	adminToken, _ := auth.GenerateAccessToken(userID, "admin@openlocalcrm.local", "ADMIN", privKey, 15*time.Minute)

	adminRoutes := []string{
		"/api/v1/users",
		"/api/v1/export/contacts.csv",
	}

	for _, route := range adminRoutes {
		// 1. BENUTZER should receive 403 Forbidden
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer "+benutzerToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for route %s with BENUTZER role, got %d", route, rec.Code)
		}

		// 2. ADMIN should be authorized (status != 401 && status != 403)
		reqAdmin := httptest.NewRequest(http.MethodGet, route, nil)
		reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
		recAdmin := httptest.NewRecorder()
		r.ServeHTTP(recAdmin, reqAdmin)
		if recAdmin.Code == http.StatusForbidden || recAdmin.Code == http.StatusUnauthorized {
			t.Errorf("expected access allowed for route %s with ADMIN role, got %d", route, recAdmin.Code)
		}
	}
}

