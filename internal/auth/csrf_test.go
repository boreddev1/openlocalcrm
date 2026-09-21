package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestCSRF_DoubleSubmitPassesWithMatchingTokens(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "valid-csrf-token"})
	req.Header.Set(auth.CSRFHeaderName, "valid-csrf-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCSRF_FailsWhenMissingHeader(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "valid-csrf-token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRF_FailsWhenMismatchedHeader(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "valid-csrf-token"})
	req.Header.Set(auth.CSRFHeaderName, "wrong-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRF_FailsWhenMissingCookie(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.Header.Set(auth.CSRFHeaderName, "some-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRF_SkipsWhenAuthViaBearerInContext(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	ctx := auth.WithAuthViaBearer(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when auth_via_bearer is true in context, got %d", rec.Code)
	}
}

func TestCSRF_DoesNotSkipOnBearerHeaderWithoutContext(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.Header.Set("Authorization", "Bearer unvalidated-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when Bearer header is present without verified context, got %d", rec.Code)
	}
}

func TestClearCSRFCookie_SetsSecure(t *testing.T) {
	rec := httptest.NewRecorder()
	auth.ClearCSRFCookie(rec)

	cookies := rec.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.CSRFCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("expected csrf cookie to be set for clearing")
	}
	if !csrfCookie.Secure {
		t.Errorf("expected ClearCSRFCookie to set Secure=true by default")
	}
	if csrfCookie.MaxAge != -1 {
		t.Errorf("expected ClearCSRFCookie to set MaxAge=-1, got %d", csrfCookie.MaxAge)
	}
}
