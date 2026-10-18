package server_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func demoAdminID(t *testing.T, q *demo.InMemoryQuerier) [16]byte {
	t.Helper()
	admin, err := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("demo admin user missing: %v", err)
	}
	return admin.ID.Bytes
}

func totpParams(id pgtype.UUID, enabled bool) db.UpdateUserTOTPParams {
	return db.UpdateUserTOTPParams{
		ID:                  id,
		TotpSecretEncrypted: pgtype.Text{String: "encrypted-secret", Valid: enabled},
		TotpEnabled:         enabled,
		TotpLastUsedStep:    0,
	}
}

// accept-invite must be reachable without a session and without a CSRF token,
// exactly like login. A CSRF-protected registration would reject the browser's
// first request, so an invited user could never accept.
func TestAcceptInviteRoutesArePublicAndCSRFFree(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	adminUser, err := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("demo admin user missing: %v", err)
	}
	adminToken, _ := auth.GenerateAccessToken(
		uuid.UUID(adminUser.ID.Bytes), "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)

	// Admin issues an invite.
	issueBody := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)
	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(issueBody))
	issueReq.Header.Set("Authorization", "Bearer "+adminToken)
	issueReq.Header.Set("Content-Type", "application/json")
	issueRec := httptest.NewRecorder()
	r.ServeHTTP(issueRec, issueReq)
	if issueRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 from the admin invite endpoint, got %d (body: %s)", issueRec.Code, issueRec.Body.String())
	}

	var issued struct {
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(issueRec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	token := issued.InviteURL[strings.Index(issued.InviteURL, "token=")+len("token="):]
	if token == "" {
		t.Fatalf("no token in the invite link %q", issued.InviteURL)
	}

	// GET validation: no Authorization header, no CSRF cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/accept-invite?token="+token, nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET accept-invite must be public, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	// POST acceptance: still no session and still no CSRF token.
	acceptBody := []byte(`{"token":"` + token + `","password":"SuperSecret1!"}`)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", bytes.NewReader(acceptBody))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	r.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST accept-invite must be public and CSRF-exempt, got %d (body: %s)", postRec.Code, postRec.Body.String())
	}

	// The token is single use, even through the router.
	againReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", bytes.NewReader(acceptBody))
	againReq.Header.Set("Content-Type", "application/json")
	againRec := httptest.NewRecorder()
	r.ServeHTTP(againRec, againReq)
	if againRec.Code != http.StatusBadRequest {
		t.Fatalf("a reused token must be refused, got %d (body: %s)", againRec.Code, againRec.Body.String())
	}
}

func TestAcceptInvitePostIsRateLimited(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	// The login limiter allows 5 attempts per minute, so the 6th must be 429.
	for i := 0; i < 5; i++ {
		body := []byte(`{"token":"nope","password":"SuperSecret1!"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.77:9001"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: expected 400, got %d (body: %s)", i+1, rec.Code, rec.Body.String())
		}
	}

	body := []byte(`{"token":"nope","password":"SuperSecret1!"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.77:9001"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after the login burst, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// The email-template surface is admin-only CRUD plus the real generator.
func TestEmailTemplateRoutesAreAdminOnly(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	benutzerToken, _ := auth.GenerateAccessToken(
		uuid.UUID(demoAdminID(t, querier)), "u@openlocalcrm.local", "BENUTZER", 0, privKey, 15*time.Minute)
	adminToken, _ := auth.GenerateAccessToken(
		uuid.UUID(demoAdminID(t, querier)), "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/email-templates", nil)
		req.Header.Set("Authorization", "Bearer "+benutzerToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s /email-templates as BENUTZER must be 403, got %d", method, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/email-templates", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /email-templates as ADMIN must be 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestEmailTemplateCreateReadUpdateDeleteRoundTrip(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})
	adminToken, _ := auth.GenerateAccessToken(
		uuid.UUID(demoAdminID(t, querier)), "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)

	do := func(method, target, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/email-templates",
		`{"key":"pv-first-offer","name":"PV Erstangebot","subject":"Ihr Angebot","body":"Hallo {{contact_name}}","category":"vertrieb"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	// The SQL inserts locale explicitly, so the column default never fires. The
	// service must supply it.
	if created["locale"] != "de-DE" {
		t.Fatalf("an empty locale must be defaulted to de-DE by the service, got %v", created["locale"])
	}

	rec = do(http.MethodGet, "/api/v1/email-templates", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "pv-first-offer") {
		t.Fatalf("list must return the persisted template, got %s", rec.Body.String())
	}

	rec = do(http.MethodPut, "/api/v1/email-templates/pv-first-offer",
		`{"name":"PV Erstangebot v2","subject":"Ihr Angebot v2","body":"Hallo {{contact_name}}, v2","category":"vertrieb"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "v2") {
		t.Fatalf("update must persist, got %s", rec.Body.String())
	}

	rec = do(http.MethodDelete, "/api/v1/email-templates/pv-first-offer", "")
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 200 or 204, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/v1/email-templates", "")
	if strings.Contains(rec.Body.String(), "pv-first-offer") {
		t.Fatalf("delete must remove the row, got %s", rec.Body.String())
	}
}

func TestEmailTemplateGenerationNeverReturnsCannedText(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	// No AI provider is reachable, so the gateway must report the upstream
	// failure instead of inventing a template.
	r := server.NewRouter(server.Config{
		DB:         querier,
		SSEHub:     sse.NewHub(),
		PubKey:     pubKey,
		PrivKey:    privKey,
		DemoMode:   true,
		AIProvider: "anthropic",
	})
	adminToken, _ := auth.GenerateAccessToken(
		uuid.UUID(demoAdminID(t, querier)), "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
		strings.NewReader(`{"prompt":"Erstelle eine Nachfass-Mail"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an unreachable provider must be an honest 502, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "{{contact_name}}") {
		t.Fatalf("a failed generation must not return a canned template: %s", rec.Body.String())
	}
}

func TestEmailTemplateGenerationRejectsAnEmptyPrompt(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})
	adminToken, _ := auth.GenerateAccessToken(
		uuid.UUID(demoAdminID(t, querier)), "admin@openlocalcrm.local", "ADMIN", 0, privKey, 15*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
		strings.NewReader(`{"prompt":"   "}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty prompt must be a 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// /me must carry the real totp_enabled from the database user row.
func TestMeTotpEnabledComesFromTheDatabaseUser(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	adminUser, err := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("demo admin user missing: %v", err)
	}
	adminToken, _ := auth.GenerateAccessToken(uuid.UUID(adminUser.ID.Bytes), adminUser.Email, "ADMIN", 0, privKey, 15*time.Minute)

	fetch := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /me: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode /me: %v", err)
		}
		return body
	}

	if got := fetch()["totp_enabled"]; got != false {
		t.Fatalf("a user without 2FA must report totp_enabled:false, got %v", got)
	}

	// Flip the stored flag and prove the next /me reflects the database, not
	// whatever the previous request said.
	if err := querier.UpdateUserTOTP(context.Background(), totpParams(adminUser.ID, true)); err != nil {
		t.Fatalf("UpdateUserTOTP failed: %v", err)
	}
	if got := fetch()["totp_enabled"]; got != true {
		t.Fatalf("/me must read totp_enabled from the database, got %v", got)
	}

	if err := querier.UpdateUserTOTP(context.Background(), totpParams(adminUser.ID, false)); err != nil {
		t.Fatalf("UpdateUserTOTP failed: %v", err)
	}
	if got := fetch()["totp_enabled"]; got != false {
		t.Fatalf("/me must follow the database back to false, got %v", got)
	}
}

// Under SP8a, a token for a user that no longer exists in the database must be
// rejected with 401 Unauthorized by AuthMiddleware (fail closed).
func TestMeTotpEnabledIsNullWhenTheUserRowIsMissing(t *testing.T) {
	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	querier := demo.NewInMemoryQuerier()
	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	ghostToken, _ := auth.GenerateAccessToken(uuid.UUID([16]byte{7, 7, 7, 7}), "ghost@example.de", "ADMIN", 0, privKey, 15*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+ghostToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /me for missing user: expected 401, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}
