package server_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestCSRFProtectionMiddleware(t *testing.T) {
	handler := server.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	// 1. GET requests pass without CSRF token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/contacts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET to pass, got %d", rec.Code)
	}

	// 2. Verified Bearer token in context passes without CSRF token
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req = req.WithContext(auth.WithAuthViaBearer(req.Context()))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected verified Bearer POST to pass, got %d", rec.Code)
	}

	// 3. Mutating POST with cookie but without X-CSRF-Token header fails (403)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: "secret-token-123"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing header, got %d", rec.Code)
	}

	// 4. Mutating POST with mismatched X-CSRF-Token header fails (403)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: "secret-token-123"})
	req.Header.Set(server.CSRFHeaderName, "wrong-token-456")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mismatched token, got %d", rec.Code)
	}

	// 5. Mutating POST with matching X-CSRF-Token header passes (200)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: "secret-token-123"})
	req.Header.Set(server.CSRFHeaderName, "secret-token-123")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching token, got %d", rec.Code)
	}

	// 6. Login endpoint passes without CSRF token
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/v1/auth/login, got %d", rec.Code)
	}
}

func TestCSRF_InvalidBearerHeaderDoesNotBypassDoubleSubmit(t *testing.T) {
	handler := server.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	// Mutating POST with cookie and Authorization: Bearer invalid (without valid context or CSRF header)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: "secret-token-123"})
	req.Header.Set("Authorization", "Bearer invalid-unauthenticated-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Bearer header without authenticated bearer context, got %d", rec.Code)
	}
}

func TestCSRF_ConnectorWebhookExempt(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keys: %v", err)
	}
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	body := []byte(`{"first_name":"Max","last_name":"Mustermann","email":"max@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/lead-intake", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer demo-connector-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("expected connector webhook to succeed outside CSRF group, got %d: %s", rec.Code, rec.Body.String())
	}
}
