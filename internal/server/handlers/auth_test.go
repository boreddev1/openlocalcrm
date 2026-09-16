package handlers_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
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
