package server_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestDemoMode_FullSecurityAndE2E(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	querier := demo.NewInMemoryQuerier()
	sseHub := sse.NewHub()

	router := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sseHub,
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	// 1. Health check reports demo_mode: true
	res, err := client.Get(ts.URL + "/api/v1/health")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v, code: %d", err, res.StatusCode)
	}
	var health map[string]any
	_ = json.NewDecoder(res.Body).Decode(&health)
	if health["demo_mode"] != true {
		t.Fatalf("expected demo_mode: true, got %v", health["demo_mode"])
	}

	// 2. Unauthenticated access to /events/stream is blocked with 401
	res, err = client.Get(ts.URL + "/events/stream")
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated SSE stream, got %d", res.StatusCode)
	}

	// 3. Login with wrong password fails
	badLoginPayload := []byte(`{"email":"admin@openlocalcrm.local","password":"wrongpassword"}`)
	res, err = client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(badLoginPayload))
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", res.StatusCode)
	}

	// 4. Login as Admin succeeds with demo123
	adminLoginPayload := []byte(`{"email":"admin@openlocalcrm.local","password":"demo123"}`)
	res, err = client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(adminLoginPayload))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for admin login, got %d", res.StatusCode)
	}
	var loginData struct {
		User struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	_ = json.NewDecoder(res.Body).Decode(&loginData)
	if loginData.User.Role != "ADMIN" {
		t.Fatalf("invalid login response: %+v", loginData)
	}
	var adminToken string
	for _, c := range res.Cookies() {
		if c.Name == "access_token" {
			adminToken = c.Value
			break
		}
	}
	if adminToken == "" {
		t.Fatalf("expected access_token cookie from login")
	}

	// 4b. Legacy alias login (admin@mavalio.local) also succeeds
	legacyPayload := []byte(`{"email":"admin@mavalio.local","password":"demo123"}`)
	legacyRes, err := client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(legacyPayload))
	if err != nil || legacyRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for legacy admin@mavalio.local login, got %d", legacyRes.StatusCode)
	}

	// 5. Authenticated SSE stream succeeds with Bearer token
	streamReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/events/stream", nil)
	streamReq.Header.Set("Authorization", "Bearer "+adminToken)

	streamRes, err := client.Do(streamReq)
	if err != nil || streamRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated SSE stream, got %d, err: %v", streamRes.StatusCode, err)
	}
	streamRes.Body.Close()

	// 5b. Finding #26: Query parameter token is rejected
	streamReqQuery, _ := http.NewRequest(http.MethodGet, ts.URL+"/events/stream?token="+adminToken, nil)
	queryRes, err := client.Do(streamReqQuery)
	if err != nil || queryRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for SSE stream with query param token, got %d", queryRes.StatusCode)
	}
	queryRes.Body.Close()

	// 6. Login as Sales User (BENUTZER)
	salesLoginPayload := []byte(`{"email":"vertrieb@mavalio.local","password":"demo123"}`)
	res, err = client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(salesLoginPayload))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for sales login, got %d", res.StatusCode)
	}
	var salesToken string
	for _, c := range res.Cookies() {
		if c.Name == "access_token" {
			salesToken = c.Value
			break
		}
	}
	if salesToken == "" {
		t.Fatalf("expected access_token cookie from sales login")
	}

	// 7. RBAC Check: Sales user attempts to DELETE contact -> 403 Forbidden
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/contacts/44444444-4444-4444-4444-444444444441", nil)
	delReq.Header.Set("Authorization", "Bearer "+salesToken)
	delRes, err := client.Do(delReq)
	if err != nil || delRes.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for sales user deleting contact, got %d", delRes.StatusCode)
	}

	// 8. Admin deletes contact -> 200 OK
	adminDelReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/contacts/44444444-4444-4444-4444-444444444441", nil)
	adminDelReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminDelRes, err := client.Do(adminDelReq)
	if err != nil || adminDelRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for admin deleting contact, got %d", adminDelRes.StatusCode)
	}

	// 9. Backup drill as admin: demo mode has no PostgreSQL pool, so a real
	// scratch-schema round trip is impossible. The endpoint must say so (501)
	// instead of reporting a healthy check it never performed.
	drillReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/backup/drill", nil)
	drillReq.Header.Set("Authorization", "Bearer "+adminToken)
	drillRes, err := client.Do(drillReq)
	if err != nil {
		t.Fatalf("backup drill request failed: %v", err)
	}
	if drillRes.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501 for a demo-mode backup drill, got %d", drillRes.StatusCode)
	}
	drillBody, _ := io.ReadAll(drillRes.Body)
	if bytes.Contains(drillBody, []byte(`"passed":true`)) || bytes.Contains(drillBody, []byte("healthy")) {
		t.Fatalf("demo drill must not claim success: %s", drillBody)
	}

	// 10. Export still works in demo mode: it streams the in-memory fixtures
	// through the same row source without buffering a whole table.
	exportReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/backup/export", nil)
	exportReq.Header.Set("Authorization", "Bearer "+adminToken)
	exportRes, err := client.Do(exportReq)
	if err != nil || exportRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for backup export, got %d", exportRes.StatusCode)
	}
	var envelope struct {
		Format string                       `json:"format"`
		Tables map[string][]json.RawMessage `json:"tables"`
	}
	if err := json.NewDecoder(exportRes.Body).Decode(&envelope); err != nil {
		t.Fatalf("backup export is not a valid envelope: %v", err)
	}
	if envelope.Format != "openlocalcrm-backup" || len(envelope.Tables) != 24 {
		t.Fatalf("unexpected backup envelope: format=%q tables=%d", envelope.Format, len(envelope.Tables))
	}

	// 11. Restore needs a pool too, so it reports 501 rather than pretending.
	restoreReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/backup/restore",
		bytes.NewReader([]byte(`{"format":"openlocalcrm-backup","version":1,"tables":{}}`)))
	restoreReq.Header.Set("Authorization", "Bearer "+adminToken)
	restoreReq.Header.Set("X-CSRF-Token", "missing-csrf-is-not-the-point")
	restoreRes, err := client.Do(restoreReq)
	if err != nil {
		t.Fatalf("backup restore request failed: %v", err)
	}
	if restoreRes.StatusCode == http.StatusOK {
		t.Fatalf("demo restore must never report success")
	}
}
