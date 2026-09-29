package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestCSRF_DoubleSubmitPassesWithMatchingTokens(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	validToken, err := auth.GenerateCSRFToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: validToken})
	req.Header.Set(auth.CSRFHeaderName, validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCSRF_FailsWhenMissingHeader(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	validToken, err := auth.GenerateCSRFToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: validToken})
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

	validToken1, _ := auth.GenerateCSRFToken()
	validToken2, _ := auth.GenerateCSRFToken()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: validToken1})
	req.Header.Set(auth.CSRFHeaderName, validToken2)
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

	validToken, _ := auth.GenerateCSRFToken()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	req.Header.Set(auth.CSRFHeaderName, validToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRF_RejectsForgedOrUnsignedTokens(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, badToken := range []string{
		"plain-token-without-hmac",
		"nonce.invalidhmacsignature1234567890abcdef",
		"malformed..token",
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
		req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: badToken})
		req.Header.Set(auth.CSRFHeaderName, badToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("token %q: expected 403, got %d", badToken, rec.Code)
		}
	}
}

func TestCSRF_SessionBoundTokens(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userA := uuid.New()
	userB := uuid.New()

	tokenA, err := auth.GenerateBoundCSRFToken(userA.String())
	if err != nil {
		t.Fatalf("failed to generate bound token: %v", err)
	}

	// 1. User A uses tokenA -> Success
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	reqA.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: tokenA})
	reqA.Header.Set(auth.CSRFHeaderName, tokenA)
	ctxA := auth.WithUserClaims(reqA.Context(), &auth.AccessClaims{UserID: userA})
	reqA = reqA.WithContext(ctxA)

	recA := httptest.NewRecorder()
	handler.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 for user A, got %d: %s", recA.Code, recA.Body.String())
	}

	// 2. User B tries to use tokenA -> Rejected
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", nil)
	reqB.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: tokenA})
	reqB.Header.Set(auth.CSRFHeaderName, tokenA)
	ctxB := auth.WithUserClaims(reqB.Context(), &auth.AccessClaims{UserID: userB})
	reqB = reqB.WithContext(ctxB)

	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when User B presents User A's CSRF token, got %d", recB.Code)
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
