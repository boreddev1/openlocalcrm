package server_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestProductionMode_NoFakeData(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	// Clean empty querier without any demo records
	cleanQuerier := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	// Seed single initial admin user (like bootstrapAdminUser does in production)
	pwHash, _ := auth.HashPassword("securepass123")
	adminUser, err := cleanQuerier.CreateUser(ctx, db.CreateUserParams{
		Email:        "admin@company.com",
		PasswordHash: pwHash,
		FirstName:    "Admin",
		LastName:     "User",
		Role:         "ADMIN",
		Status:       "ACTIVE",
	})
	if err != nil {
		t.Fatalf("failed creating admin user: %v", err)
	}

	sseHub := sse.NewHub()
	router := server.NewRouter(server.Config{
		DB:       cleanQuerier,
		SSEHub:   sseHub,
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: false, // Production mode!
	})

	ts := httptest.NewServer(router)
	defer ts.Close()
	client := ts.Client()

	// 1. Health check reports demo_mode: false
	healthRes, err := client.Get(ts.URL + "/api/v1/health")
	if err != nil || healthRes.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v, status: %d", err, healthRes.StatusCode)
	}
	var health map[string]any
	_ = json.NewDecoder(healthRes.Body).Decode(&health)
	if health["demo_mode"] != false {
		t.Fatalf("expected demo_mode: false in production, got %v", health["demo_mode"])
	}

	// 2. Login as admin
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    adminUser.Email,
		"password": "securepass123",
	})
	loginRes, err := client.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	if err != nil || loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %v, status: %d", err, loginRes.StatusCode)
	}
	var loginData struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(loginRes.Body).Decode(&loginData)
	token := loginData.Token

	reqWithAuth := func(method, path string) *http.Request {
		req, _ := http.NewRequest(method, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	// 3. Contacts: must be empty []
	res, err := client.Do(reqWithAuth(http.MethodGet, "/api/v1/contacts"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get contacts failed: %v, status: %d", err, res.StatusCode)
	}
	var contacts []any
	_ = json.NewDecoder(res.Body).Decode(&contacts)
	if len(contacts) != 0 {
		t.Fatalf("expected 0 contacts in clean production, got %d", len(contacts))
	}

	// 4. Deals: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/deals"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get deals failed: %v, status: %d", err, res.StatusCode)
	}
	var deals []any
	_ = json.NewDecoder(res.Body).Decode(&deals)
	if len(deals) != 0 {
		t.Fatalf("expected 0 deals in clean production, got %d", len(deals))
	}

	// 5. Companies: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/companies"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get companies failed: %v, status: %d", err, res.StatusCode)
	}
	var companies []any
	_ = json.NewDecoder(res.Body).Decode(&companies)
	if len(companies) != 0 {
		t.Fatalf("expected 0 companies in clean production, got %d", len(companies))
	}

	// 6. Todos: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/todos"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get todos failed: %v, status: %d", err, res.StatusCode)
	}
	var todos []any
	_ = json.NewDecoder(res.Body).Decode(&todos)
	if len(todos) != 0 {
		t.Fatalf("expected 0 todos in clean production, got %d", len(todos))
	}

	// 7. Notes: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/notes"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get notes failed: %v, status: %d", err, res.StatusCode)
	}
	var notes []any
	_ = json.NewDecoder(res.Body).Decode(&notes)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes in clean production, got %d", len(notes))
	}

	// 8. Appointments: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/appointments"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get appointments failed: %v, status: %d", err, res.StatusCode)
	}
	var appointments []any
	_ = json.NewDecoder(res.Body).Decode(&appointments)
	if len(appointments) != 0 {
		t.Fatalf("expected 0 appointments in clean production, got %d", len(appointments))
	}

	// 9. Sales Reports: must have empty forecast and zero stats
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/reports/sales"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get sales reports failed: %v, status: %d", err, res.StatusCode)
	}
	var report struct {
		Forecast        []any `json:"forecast"`
		ConversionStats struct {
			TotalLeads       int     `json:"total_leads"`
			WonDeals         int     `json:"won_deals"`
			LostDeals        int     `json:"lost_deals"`
			RevokedDeals     int     `json:"revoked_deals"`
			ConversionRate   float64 `json:"conversion_rate"`
			AvgDealVolumeEUR float64 `json:"avg_deal_volume_eur"`
		} `json:"conversion_stats"`
	}
	_ = json.NewDecoder(res.Body).Decode(&report)
	if len(report.Forecast) != 0 {
		t.Fatalf("expected empty forecast in clean production, got %d items", len(report.Forecast))
	}
	if report.ConversionStats.TotalLeads != 0 || report.ConversionStats.WonDeals != 0 ||
		report.ConversionStats.ConversionRate != 0 || report.ConversionStats.AvgDealVolumeEUR != 0 {
		t.Fatalf("expected zero conversion stats in clean production, got %+v", report.ConversionStats)
	}

	// 10. AI Knowledge Base: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/ai/kb"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get kb articles failed: %v, status: %d", err, res.StatusCode)
	}
	var kbArticles []any
	_ = json.NewDecoder(res.Body).Decode(&kbArticles)
	if len(kbArticles) != 0 {
		t.Fatalf("expected 0 KB articles in clean production, got %d", len(kbArticles))
	}

	// 11. AI Research Jobs: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/ai/research/jobs"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get research jobs failed: %v, status: %d", err, res.StatusCode)
	}
	var researchJobs []any
	_ = json.NewDecoder(res.Body).Decode(&researchJobs)
	if len(researchJobs) != 0 {
		t.Fatalf("expected 0 research jobs in clean production, got %d", len(researchJobs))
	}

	// 12. Automation Runs: must be empty []
	res, err = client.Do(reqWithAuth(http.MethodGet, "/api/v1/automations/runs"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get automation runs failed: %v, status: %d", err, res.StatusCode)
	}
	var runs []any
	_ = json.NewDecoder(res.Body).Decode(&runs)
	if len(runs) != 0 {
		t.Fatalf("expected 0 automation runs in clean production, got %d", len(runs))
	}

	// 13. Backup Drill: confirms 0 companies, 0 contacts, 0 deals
	res, err = client.Do(reqWithAuth(http.MethodPost, "/api/v1/backup/drill"))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("backup drill failed: %v, status: %d", err, res.StatusCode)
	}
	var drill struct {
		CompanyCount int `json:"company_count"`
		ContactCount int `json:"contact_count"`
		DealCount    int `json:"deal_count"`
	}
	_ = json.NewDecoder(res.Body).Decode(&drill)
	if drill.CompanyCount != 0 || drill.ContactCount != 0 || drill.DealCount != 0 {
		t.Fatalf("expected 0 counts in backup drill, got %+v", drill)
	}
}
