package server_test

import (
	"bytes"
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
	if resp["ai_model"] == nil || resp["ai_model"] == "" {
		t.Errorf("expected non-empty ai_model in health check, got %v", resp["ai_model"])
	}
	if resp["ai_provider"] == nil || resp["ai_provider"] == "" {
		t.Errorf("expected non-empty ai_provider in health check, got %v", resp["ai_provider"])
	}

	// Custom model test
	rCustom := server.NewRouter(server.Config{
		SSEHub:     sse.NewHub(),
		PubKey:     pubKey,
		PrivKey:    privKey,
		AIProvider: "ollama",
		AIModel:    "mistral",
	})
	reqCustom := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	recCustom := httptest.NewRecorder()
	rCustom.ServeHTTP(recCustom, reqCustom)
	var respCustom map[string]any
	_ = json.NewDecoder(recCustom.Body).Decode(&respCustom)
	if respCustom["ai_model"] != "mistral" {
		t.Errorf("expected mistral model, got %v", respCustom["ai_model"])
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

	userID := uuid.New()
	adminToken, _ := auth.GenerateAccessToken(userID, "admin@openlocalcrm.local", "ADMIN", privKey, 15*time.Minute)

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
