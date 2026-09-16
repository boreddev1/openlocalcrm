package server_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
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
		Token string `json:"token"`
		User  struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	_ = json.NewDecoder(res.Body).Decode(&loginData)
	if loginData.Token == "" || loginData.User.Role != "ADMIN" {
		t.Fatalf("invalid login response: %+v", loginData)
	}
	adminToken := loginData.Token

	// 4b. Legacy alias login (admin@mavalio.local) also succeeds
	legacyPayload := []byte(`{"email":"admin@mavalio.local","password":"demo123"}`)
	legacyRes, err := client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(legacyPayload))
	if err != nil || legacyRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for legacy admin@mavalio.local login, got %d", legacyRes.StatusCode)
	}

	// 5. Authenticated SSE stream succeeds with Bearer token
	streamReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/events/stream", nil)
	streamReq.Header.Set("Authorization", "Bearer "+adminToken)
	// Or test via query parameter
	streamReqQuery, _ := http.NewRequest(http.MethodGet, ts.URL+"/events/stream?token="+adminToken, nil)

	streamRes, err := client.Do(streamReqQuery)
	if err != nil || streamRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated SSE stream, got %d, err: %v", streamRes.StatusCode, err)
	}
	streamRes.Body.Close()

	// 6. Login as Sales User (BENUTZER)
	salesLoginPayload := []byte(`{"email":"vertrieb@mavalio.local","password":"demo123"}`)
	res, err = client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(salesLoginPayload))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for sales login, got %d", res.StatusCode)
	}
	var salesData struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(res.Body).Decode(&salesData)
	salesToken := salesData.Token

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

	// 9. Backup drill as admin succeeds
	drillReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/backup/drill", nil)
	drillReq.Header.Set("Authorization", "Bearer "+adminToken)
	drillRes, err := client.Do(drillReq)
	if err != nil || drillRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for backup drill, got %d", drillRes.StatusCode)
	}
}
