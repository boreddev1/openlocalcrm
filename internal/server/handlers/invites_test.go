package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

// --- admin issuance ---------------------------------------------------------

func TestUserHandlerInviteReturnsTheOneTimeLink(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewUserHandler(q, svc)

	adminUser, err := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("demo admin user missing: %v", err)
	}
	adminClaims := &auth.AccessClaims{
		UserID: uuid.UUID(adminUser.ID.Bytes),
		Email:  "admin@openlocalcrm.local",
		Role:   "ADMIN",
	}

	payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.Invite(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode invite response: %v (body: %s)", err, rec.Body.String())
	}

	link, _ := body["invite_url"].(string)
	if !strings.Contains(link, "/accept-invite?token=") {
		t.Fatalf("the response must return a copyable /accept-invite link, got %v", body["invite_url"])
	}
	if strings.HasPrefix(link, "/api") {
		t.Fatalf("the link must be a browser link, not an API path, got %q", link)
	}
	if body["expires_at"] == nil || body["expires_at"] == "" {
		t.Fatalf("the response must state when the invite expires, got %v", body)
	}

	user, _ := body["user"].(map[string]any)
	if user == nil {
		t.Fatalf("the response must include the created user, got %v", body)
	}
	if user["status"] != "INVITED" {
		t.Fatalf("the created user must be INVITED, got %v", user["status"])
	}
	if user["totp_enabled"] != false {
		t.Fatalf("totp_enabled must come from the stored user, got %v", user["totp_enabled"])
	}

	// The response must not claim an email was sent: nothing in this codebase
	// can deliver mail for an invite, so the claim would be a lie.
	raw := rec.Body.String()
	for _, lie := range []string{"gesendet", "email_sent", "\"sent\"", "E-Mail gesendet"} {
		if strings.Contains(raw, lie) {
			t.Fatalf("the invite response must not claim an email was sent (%q): %s", lie, raw)
		}
	}

	// The raw token is only ever in the link. The stored invite keeps the hash.
	token := link[strings.Index(link, "token=")+len("token="):]
	if strings.Contains(raw, "token_hash") {
		t.Fatalf("the invite response must never expose token_hash: %s", raw)
	}
	if strings.Count(raw, token) != 1 {
		t.Fatalf("the raw token must appear exactly once, in the link: %s", raw)
	}
	if token == "" {
		t.Fatal("the returned link must carry a token")
	}
}

func TestUserHandlerInviteStillProtectsNonAdminsAndRejectsBadInput(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewUserHandler(q, svc)

	benutzerClaims := &auth.AccessClaims{UserID: uuid.New(), Email: "u@openlocalcrm.local", Role: "BENUTZER"}
	payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, benutzerClaims))
	rec := httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-admin invite, got %d", rec.Code)
	}

	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminClaims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"malformed json": {body: `{`, wantErr: "invalid_request"},
		"missing email":  {body: `{"first_name":"Tim","last_name":"Tester"}`, wantErr: "validation_error"},
		"bad email":      {body: `{"email":"not-an-email","first_name":"Tim","last_name":"Tester"}`, wantErr: "validation_error"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", strings.NewReader(tc.body))
			req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
			rec := httptest.NewRecorder()
			h.Invite(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Fatalf("expected %q in the body, got %s", tc.wantErr, rec.Body.String())
			}
		})
	}
}

func TestUserHandlerInviteNormalizesTheEmail(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := handlers.NewUserHandler(q, invite.NewService(q, nil))
	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminClaims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

	payload := []byte(`{"email":"  TIM@Example.DE ","first_name":"Tim","last_name":"Tester","role":"vertrieb"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	user, err := q.GetUserByEmail(context.Background(), "tim@example.de")
	if err != nil {
		t.Fatalf("the email must be trimmed and lower-cased before insert: %v", err)
	}
	if user.Role != "VERTRIEB" {
		t.Fatalf("a lower-case role must be accepted, got %q", user.Role)
	}
}

func TestUserHandlerInviteRejectsAnUnknownRoleRatherThanSilentlyDowngrading(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := handlers.NewUserHandler(q, invite.NewService(q, nil))
	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminClaims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

	payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"ROOT"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.Invite(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown role must be refused instead of silently downgraded, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_role") {
		t.Fatalf("expected invalid_role in the body, got %s", rec.Body.String())
	}
}

// --- invite link origin ------------------------------------------------------

// TestInviteLinkOriginFollowsTheRequest drives the whole production path — the
// admin handler, inviteBaseURL and BuildInviteURL — because that is the only
// place the origin is decided. A unit test on the URL builder alone would cover
// parameters production never fills in.
//
// httptest.NewRequest gives a request a non-nil r.TLS for an https:// target and
// a nil r.TLS for an http:// one, which is exactly the distinction the scheme
// logic has to get right.
func TestInviteLinkOriginFollowsTheRequest(t *testing.T) {
	cases := []struct {
		name string
		// target is the request URL; its scheme decides whether r.TLS is set.
		target    string
		host      string
		forwarded string
		want      string
	}{
		{
			name:   "direct tls is not downgraded to plaintext",
			target: "https://crm.example.de/api/v1/users/invite",
			want:   "https://crm.example.de/accept-invite?token=",
		},
		{
			name:      "forwarded https over a direct tls connection",
			target:    "https://crm.example.de/api/v1/users/invite",
			forwarded: "https",
			want:      "https://crm.example.de/accept-invite?token=",
		},
		{
			name:      "forwarded http is honored",
			target:    "http://crm.example.de/api/v1/users/invite",
			forwarded: "http",
			want:      "http://crm.example.de/accept-invite?token=",
		},
		{
			name:      "forwarded https on a plaintext hop",
			target:    "http://crm.example.de/api/v1/users/invite",
			forwarded: "https",
			want:      "https://crm.example.de/accept-invite?token=",
		},
		{
			name:      "only the first forwarded hop counts",
			target:    "http://crm.example.de/api/v1/users/invite",
			forwarded: "https, http",
			want:      "https://crm.example.de/accept-invite?token=",
		},
		{
			name:      "an empty forwarded value falls back to https",
			target:    "https://crm.example.de/api/v1/users/invite",
			forwarded: "",
			want:      "https://crm.example.de/accept-invite?token=",
		},
		{
			name:      "nothing indicates plaintext so https is assumed",
			target:    "http://crm.example.de/api/v1/users/invite",
			forwarded: "",
			want:      "https://crm.example.de/accept-invite?token=",
		},
		{
			name:   "a hostile Host header degrades to a relative link",
			target: "https://crm.example.de/api/v1/users/invite",
			host:   "evil.example/\r\nX-Injected: 1",
			want:   "/accept-invite?token=",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The configured DOMAIN must not mask the request-derived origin.
			t.Setenv("DOMAIN", "")

			q := demo.NewInMemoryQuerier()
			h := handlers.NewUserHandler(q, invite.NewService(q, nil))
			adminUser, err := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
			if err != nil {
				t.Fatalf("demo admin user missing: %v", err)
			}
			claims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

			payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)
			req := httptest.NewRequest(http.MethodPost, tc.target, bytes.NewReader(payload))
			if tc.host != "" {
				req.Host = tc.host
			}
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwarded)
			} else {
				// An explicitly empty header is not the same as an absent one:
				// the fallback must not be short-circuited by another branch.
				req.Header.Set("X-Forwarded-Proto", "")
			}
			req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, claims))

			rec := httptest.NewRecorder()
			h.Invite(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
			}
			var body struct {
				InviteURL string `json:"invite_url"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode invite response: %v", err)
			}
			if !strings.HasPrefix(body.InviteURL, tc.want) {
				t.Fatalf("invite_url = %q, want prefix %q", body.InviteURL, tc.want)
			}
			token := strings.TrimPrefix(body.InviteURL, tc.want)
			if token == "" {
				t.Fatalf("the link must carry a token, got %q", body.InviteURL)
			}
			if strings.ContainsAny(token, " \r\n") {
				t.Fatalf("the token must be query-escaped, got %q", body.InviteURL)
			}
		})
	}
}

// TestInviteLinkOriginPrefersTheConfiguredDomain pins that DOMAIN still wins
// over anything the request says.
func TestInviteLinkOriginPrefersTheConfiguredDomain(t *testing.T) {
	t.Setenv("DOMAIN", "crm.example.de")

	q := demo.NewInMemoryQuerier()
	h := handlers.NewUserHandler(q, invite.NewService(q, nil))
	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	claims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

	payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/v1/users/invite", bytes.NewReader(payload))
	req.Header.Set("X-Forwarded-Proto", "http")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, claims))

	rec := httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		InviteURL string `json:"invite_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	if !strings.HasPrefix(body.InviteURL, "https://crm.example.de/accept-invite?token=") {
		t.Fatalf("DOMAIN must win over the request, got %q", body.InviteURL)
	}
}

// --- audit identity on the public paths --------------------------------------

type inviteAuditEntry struct {
	action     string
	entityType string
	ip         string
	userAgent  string
	changes    any
}

type inviteAuditRecorder struct {
	entries []inviteAuditEntry
}

func (a *inviteAuditRecorder) Log(_ context.Context, _ pgtype.UUID, entityType string, _ pgtype.UUID, action string, changes any, ip, userAgent string) error {
	a.entries = append(a.entries, inviteAuditEntry{
		action:     action,
		entityType: entityType,
		ip:         ip,
		userAgent:  userAgent,
		changes:    changes,
	})
	return nil
}

func (a *inviteAuditRecorder) find(action string) (inviteAuditEntry, bool) {
	for _, e := range a.entries {
		if e.action == action {
			return e, true
		}
	}
	return inviteAuditEntry{}, false
}

// TestInviteAuditRowsCarryTheRealRequestIdentity covers Important 2. The
// service already knows how to carry the request identity (RequestMeta travels in
// the context and the admin issuance path uses it), so the public paths must not
// drop it: an audit row with an empty ip_address and user_agent is a row nobody
// can investigate later.
func TestInviteAuditRowsCarryTheRealRequestIdentity(t *testing.T) {
	const wantIP = "203.0.113.9"
	const wantAgent = "Playwright Invite Agent/1.0"

	newRequest := func(method, target, body string) *http.Request {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, target, nil)
		} else {
			req = httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
		}
		// A public, non-loopback address, so auth.ClientIP reports it verbatim.
		req.RemoteAddr = wantIP + ":41234"
		req.Header.Set("User-Agent", wantAgent)
		return req
	}

	t.Run("accepted", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		auditLog := &inviteAuditRecorder{}
		svc := invite.NewService(q, auditLog)
		h := handlers.NewInviteHandler(svc)

		adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
		issued, err := svc.Issue(context.Background(), adminUser.ID, invite.CreateUserInput{
			Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
		}, "https://crm.example.de")
		if err != nil {
			t.Fatalf("Issue failed: %v", err)
		}
		auditLog.entries = nil

		body, _ := json.Marshal(map[string]string{"token": issued.RawToken, "password": "SuperSecret1!"})
		rec := httptest.NewRecorder()
		h.Accept(rec, newRequest(http.MethodPost, "/api/v1/auth/accept-invite", string(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		entry, ok := auditLog.find("USER_INVITE_ACCEPTED")
		if !ok {
			t.Fatalf("expected a USER_INVITE_ACCEPTED event, got %+v", auditLog.entries)
		}
		if entry.ip != wantIP {
			t.Fatalf("ip_address = %q, want %q — the audit row must name the real client", entry.ip, wantIP)
		}
		if entry.userAgent != wantAgent {
			t.Fatalf("user_agent = %q, want %q", entry.userAgent, wantAgent)
		}
		blob := fmt.Sprintf("%+v", entry)
		if strings.Contains(blob, issued.RawToken) || strings.Contains(blob, issued.Invite.TokenHash) {
			t.Fatalf("the audit row must never carry the token: %s", blob)
		}
	})

	t.Run("rejected validation", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		auditLog := &inviteAuditRecorder{}
		svc := invite.NewService(q, auditLog)
		h := handlers.NewInviteHandler(svc)

		bogus := "not-a-real-token"
		rec := httptest.NewRecorder()
		h.Validate(rec, newRequest(http.MethodGet, "/api/v1/auth/accept-invite?token="+bogus, ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}

		entry, ok := auditLog.find("USER_INVITE_REJECTED")
		if !ok {
			t.Fatalf("expected a USER_INVITE_REJECTED event, got %+v", auditLog.entries)
		}
		if entry.ip != wantIP {
			t.Fatalf("ip_address = %q, want %q — a rejection is exactly the row an admin investigates", entry.ip, wantIP)
		}
		if entry.userAgent != wantAgent {
			t.Fatalf("user_agent = %q, want %q", entry.userAgent, wantAgent)
		}
		blob := fmt.Sprintf("%+v", entry)
		if strings.Contains(blob, bogus) {
			t.Fatalf("the audit row must never carry the token: %s", blob)
		}
	})

	t.Run("rejected acceptance", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		auditLog := &inviteAuditRecorder{}
		svc := invite.NewService(q, auditLog)
		h := handlers.NewInviteHandler(svc)

		body, _ := json.Marshal(map[string]string{"token": "not-a-real-token", "password": "short"})
		rec := httptest.NewRecorder()
		h.Accept(rec, newRequest(http.MethodPost, "/api/v1/auth/accept-invite", string(body)))

		entry, ok := auditLog.find("USER_INVITE_REJECTED")
		if !ok {
			t.Fatalf("expected a USER_INVITE_REJECTED event, got %+v", auditLog.entries)
		}
		if entry.ip != wantIP || entry.userAgent != wantAgent {
			t.Fatalf("rejected acceptance recorded ip=%q ua=%q, want %q / %q", entry.ip, entry.userAgent, wantIP, wantAgent)
		}
	})
}

// --- public validate --------------------------------------------------------

func TestInviteHandlerValidateReturnsSafeMetadataOnly(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewInviteHandler(svc)

	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	issued, err := svc.Issue(context.Background(), adminUser.ID, invite.CreateUserInput{
		Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
	}, "https://crm.example.de")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/accept-invite?token="+issued.RawToken, nil)
	rec := httptest.NewRecorder()
	h.Validate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if body["valid"] != true {
		t.Fatalf("expected valid:true, got %v", body)
	}
	if body["email"] != "tim@example.de" {
		t.Fatalf("expected the invited email, got %v", body["email"])
	}
	if body["first_name"] != "Tim" {
		t.Fatalf("expected the invited first name, got %v", body["first_name"])
	}
	if body["expires_at"] == nil || body["expires_at"] == "" {
		t.Fatalf("expected expires_at, got %v", body)
	}

	// db.UserInvite carries token_hash, so the projection must be explicit.
	raw := rec.Body.String()
	for _, forbidden := range []string{"token_hash", issued.Invite.TokenHash, "consumed_at", "revoked_at", "created_by"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("the validate response must not expose %q: %s", forbidden, raw)
		}
	}
}

func TestInviteHandlerValidateRejectsAnUnknownOrMissingToken(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := handlers.NewInviteHandler(invite.NewService(q, nil))

	for _, target := range []string{
		"/api/v1/auth/accept-invite?token=nope",
		"/api/v1/auth/accept-invite",
		"/api/v1/auth/accept-invite?token=",
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			h.Validate(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "invite_not_usable") {
				t.Fatalf("expected the stable invite_not_usable code, got %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"valid":true`) {
				t.Fatalf("a refused validation must never report valid:true: %s", rec.Body.String())
			}
		})
	}
}

// --- public accept ----------------------------------------------------------

func TestInviteHandlerAcceptActivatesTheAccount(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewInviteHandler(svc)

	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	issued, err := svc.Issue(context.Background(), adminUser.ID, invite.CreateUserInput{
		Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
	}, "https://crm.example.de")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"token": issued.RawToken, "password": "SuperSecret1!"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Accept(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err = json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode accept response: %v", err)
	}
	if resp["status"] != "ACTIVE" {
		t.Fatalf("expected status ACTIVE, got %v", resp["status"])
	}
	if resp["email"] != "tim@example.de" {
		t.Fatalf("expected the activated email, got %v", resp["email"])
	}
	// Accepting an invite must not establish a session: login stays the only
	// session-establishing operation.
	if strings.Contains(strings.ToLower(rec.Header().Get("Set-Cookie")), "access_token") {
		t.Fatalf("POST /accept-invite must not set an auth cookie, got %q", rec.Header().Get("Set-Cookie"))
	}

	user, err := q.GetUserByID(context.Background(), issued.Invite.UserID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if user.Status != "ACTIVE" {
		t.Fatalf("the account must be ACTIVE, got %q", user.Status)
	}
	if !auth.CheckPassword(user.PasswordHash, "SuperSecret1!") {
		t.Fatal("the chosen password must be stored as a real Argon2id hash")
	}
}

func TestInviteHandlerAcceptRejectsBadPayloadsAndUnusableTokens(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewInviteHandler(svc)

	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	issued, _ := svc.Issue(context.Background(), adminUser.ID, invite.CreateUserInput{
		Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
	}, "https://crm.example.de")

	cases := map[string]struct {
		body     string
		wantCode int
		wantErr  string
	}{
		"malformed json": {body: `{`, wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		"missing token":  {body: `{"password":"SuperSecret1!"}`, wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		"missing passwd": {body: `{"token":"` + issued.RawToken + `"}`, wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		"weak password":  {body: `{"token":"` + issued.RawToken + `","password":"weak"}`, wantCode: http.StatusBadRequest, wantErr: "weak_password"},
		"unknown token":  {body: `{"token":"nope","password":"SuperSecret1!"}`, wantCode: http.StatusBadRequest, wantErr: "invite_not_usable"},
		// An oversized body is refused before any token work, so the public
		// endpoint cannot be used as a memory sink.
		"oversized body": {body: `{"token":"` + strings.Repeat("x", 8192) + `","password":"SuperSecret1!"}`, wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.Accept(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d (body: %s)", tc.wantCode, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Fatalf("expected %q in the body, got %s", tc.wantErr, rec.Body.String())
			}
		})
	}
}

func TestInviteHandlerAcceptRejectsAReusedTokenWithTheSameStableError(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := invite.NewService(q, nil)
	h := handlers.NewInviteHandler(svc)

	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	issued, _ := svc.Issue(context.Background(), adminUser.ID, invite.CreateUserInput{
		Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
	}, "https://crm.example.de")

	body := `{"token":"` + issued.RawToken + `","password":"SuperSecret1!"}`
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/accept-invite", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Accept(rec, req)

		if attempt == 0 && rec.Code != http.StatusOK {
			t.Fatalf("attempt 1: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if attempt == 1 {
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("attempt 2: expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "invite_not_usable") {
				t.Fatalf("attempt 2: expected invite_not_usable, got %s", rec.Body.String())
			}
		}
	}
}

func TestInviteHandlerAuditFailureDoesNotBreakIssuance(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	// A nil audit logger must not panic: audit is best-effort bookkeeping and
	// must never turn a real invite into a fake failure or vice versa.
	h := handlers.NewUserHandler(q, invite.NewService(q, nil))
	adminUser, _ := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminClaims := &auth.AccessClaims{UserID: uuid.UUID(adminUser.ID.Bytes), Role: "ADMIN"}

	payload := []byte(`{"email":"tim@example.de","first_name":"Tim","last_name":"Tester","role":"BENUTZER"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 even without an audit sink, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// --- projection regression --------------------------------------------------

// The list surface must never hand db.UserInvite structs to the JSON encoder.
// This test pins that the invite view type itself carries no secret field.
func TestInviteViewSerializesNoSecret(t *testing.T) {
	view := invite.InviteView{
		Email:     "tim@example.de",
		FirstName: "Tim",
		LastName:  "Tester",
		ExpiresAt: time.Date(2026, 12, 24, 10, 0, 0, 0, time.UTC),
	}
	blob, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal invite view: %v", err)
	}
	for _, forbidden := range []string{"token", "hash", "consumed", "revoked"} {
		if strings.Contains(strings.ToLower(string(blob)), forbidden) {
			t.Fatalf("InviteView must not serialize %q: %s", forbidden, blob)
		}
	}
}
