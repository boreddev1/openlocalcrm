package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/server"
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

	// 2. Bearer token POST passes without CSRF token (for webhooks / APIs)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/lead-intake", nil)
	req.Header.Set("Authorization", "Bearer valid-secret-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected Bearer POST to pass, got %d", rec.Code)
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
