package server_test

import (
	"bytes"
	"context"
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
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
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
	if resp["demo_mode"] != true {
		t.Fatalf("expected demo_mode true, got %v", resp["demo_mode"])
	}
	// F-40: Ensure internal AI configuration is NOT disclosed
	if resp["ai_model"] != nil {
		t.Errorf("expected ai_model to be omitted from health response, got %v", resp["ai_model"])
	}
	if resp["ai_provider"] != nil {
		t.Errorf("expected ai_provider to be omitted from health response, got %v", resp["ai_provider"])
	}
	if resp["ai_base_url"] != nil {
		t.Errorf("expected ai_base_url to be omitted from health response, got %v", resp["ai_base_url"])
	}
}

func TestSecurityHeaders(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	r := server.NewRouter(server.Config{
		SSEHub:  sse.NewHub(),
		PubKey:  pubKey,
		PrivKey: privKey,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	headers := rec.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing or incorrect X-Content-Type-Options header")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing or incorrect X-Frame-Options header")
	}
	if headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("missing or incorrect Referrer-Policy header")
	}
	if headers.Get("Permissions-Policy") == "" {
		t.Errorf("missing Permissions-Policy header")
	}
	if headers.Get("Content-Security-Policy") == "" {
		t.Errorf("missing Content-Security-Policy header")
	}
	if headers.Get("Strict-Transport-Security") == "" {
		t.Errorf("missing Strict-Transport-Security header on https request")
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
	token, _ := auth.GenerateAccessToken(userID, "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)
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

	adminUser, _ := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	vertriebUser, _ := querier.GetUserByEmail(context.Background(), "vertrieb@openlocalcrm.local")
	benutzerToken, _ := auth.GenerateAccessToken(uuid.UUID(vertriebUser.ID.Bytes), vertriebUser.Email, vertriebUser.Role, 0, privKey, 15*time.Minute)
	adminToken, _ := auth.GenerateAccessToken(uuid.UUID(adminUser.ID.Bytes), adminUser.Email, adminUser.Role, 0, privKey, 15*time.Minute)

	adminRoutes := []string{
		"/api/v1/users",
		"/api/v1/export/contacts.csv",
		"/api/v1/settings/export",
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

func TestSettingsEndpoints(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	adminUser, _ := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminToken, _ := auth.GenerateAccessToken(uuid.UUID(adminUser.ID.Bytes), adminUser.Email, adminUser.Role, 0, privKey, 15*time.Minute)

	// GET /api/v1/settings/export
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/export", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on settings export, got %d: %s", rec.Code, rec.Body.String())
	}

	var exported map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&exported); err != nil {
		t.Fatalf("failed decoding exported settings: %v", err)
	}

	if exported["version"] != "v1.0.3" {
		t.Errorf("expected version v1.0.3, got %v", exported["version"])
	}

	// POST /api/v1/settings/import
	importPayload := []byte(`{
		"version": "v1.0.3",
		"ai": {
			"provider": "ollama",
			"ollama_base_url": "http://localhost:11434",
			"ollama_model": "mistral:latest"
		},
		"admin": {
			"email": "imported-admin@openlocalcrm.local",
			"name": "Imported Admin"
		},
		"system": {
			"port": 8080
		}
	}`)
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader(importPayload))
	importReq.Header.Set("Authorization", "Bearer "+adminToken)
	importReq.Header.Set("Content-Type", "application/json")
	importRec := httptest.NewRecorder()
	r.ServeHTTP(importRec, importReq)

	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on settings import, got %d: %s", importRec.Code, importRec.Body.String())
	}
}

func TestLoginRateLimit_TriggeredAfter5Attempts(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	for i := 0; i < 5; i++ {
		payload := []byte(`{"email":"admin@openlocalcrm.local","password":"badpassword"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.25:4567"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 Unauthorized, got %d", i+1, rec.Code)
		}
	}

	// 6th attempt must be rate-limited with 429 Too Many Requests
	payload := []byte(`{"email":"admin@openlocalcrm.local","password":"badpassword"}`)
	req6 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req6.Header.Set("Content-Type", "application/json")
	req6.RemoteAddr = "198.51.100.25:4567"
	rec6 := httptest.NewRecorder()
	r.ServeHTTP(rec6, req6)

	if rec6.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 6th login attempt, got %d: %s", rec6.Code, rec6.Body.String())
	}
}
