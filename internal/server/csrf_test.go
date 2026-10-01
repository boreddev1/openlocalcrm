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

	validToken, err := auth.GenerateCSRFToken()
	if err != nil {
		t.Fatalf("failed generating token: %v", err)
	}

	// 3. Mutating POST with cookie but without X-CSRF-Token header fails (403)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: validToken})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing header, got %d", rec.Code)
	}

	// 4. Mutating POST with mismatched X-CSRF-Token header fails (403)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: validToken})
	req.Header.Set(server.CSRFHeaderName, "wrong-token-456")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mismatched token, got %d", rec.Code)
	}

	// 5. Mutating POST with matching X-CSRF-Token header passes (200)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: validToken})
	req.Header.Set(server.CSRFHeaderName, validToken)
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

	validToken, _ := auth.GenerateCSRFToken()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: validToken})
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

func TestCSRF_RefreshAndLogoutWorkWithoutAccessToken(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating keys: %v", err)
	}
	router := server.NewRouter(server.Config{
		DB:       demo.NewInMemoryQuerier(),
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"email":"admin@openlocalcrm.local","password":"demo123"}`)))
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}

	// Simulate an expired session: the browser dropped access_token, refresh/csrf cookies remain.
	var refresh, csrf *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		switch c.Name {
		case "refresh_token":
			refresh = c
		case "csrf_token":
			csrf = c
		}
	}
	if refresh == nil || csrf == nil {
		t.Fatalf("login must set refresh_token and csrf_token cookies")
	}

	call := func(path string, withHeader bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(refresh)
		req.AddCookie(csrf)
		if withHeader {
			req.Header.Set("X-CSRF-Token", csrf.Value)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := call("/api/v1/auth/refresh", false); rec.Code != http.StatusForbidden {
		t.Fatalf("refresh without CSRF header must be 403, got %d", rec.Code)
	}
	rec := call("/api/v1/auth/refresh", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh with CSRF pair must be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Rotation: pick up the new refresh cookie for the logout call.
	for _, c := range rec.Result().Cookies() {
		if c.Name == "refresh_token" {
			refresh = c
		}
		if c.Name == "csrf_token" {
			csrf = c
		}
	}
	if rec := call("/api/v1/auth/logout", true); rec.Code != http.StatusOK {
		t.Fatalf("logout with CSRF pair must be 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
