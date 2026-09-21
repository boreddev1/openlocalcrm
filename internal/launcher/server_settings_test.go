package launcher

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerSettingsEndpoints(t *testing.T) {
	tmp := t.TempDir()
	envContent := `PORT=8080
INITIAL_ADMIN_EMAIL=admin@solar.de
INITIAL_ADMIN_PASSWORD=MySecretPassword123!
AI_PROVIDER=ollama
OLLAMA_BASE_URL=http://host.docker.internal:11434
OLLAMA_MODEL=gemma4:12b
OPENLOCALCRM_VERSION=v1.0.2
`
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(envContent), 0600)

	engine := NewEngine(tmp)
	server := NewServer(tmp, engine)

	t.Run("GET /api/settings/export", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings/export", nil)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var exported ExportedSettings
		if err := json.NewDecoder(rec.Body).Decode(&exported); err != nil {
			t.Fatalf("failed decoding exported JSON: %v", err)
		}

		if exported.AI.Provider != "ollama" {
			t.Errorf("expected ollama, got %s", exported.AI.Provider)
		}
		if exported.Admin.Email != "admin@solar.de" {
			t.Errorf("expected admin@solar.de, got %s", exported.Admin.Email)
		}
		if exported.System.Port != 8080 {
			t.Errorf("expected port 8080, got %d", exported.System.Port)
		}
	})

	t.Run("POST /api/settings/import via JSON body", func(t *testing.T) {
		newSettings := ExportedSettings{
			AI: AISettings{
				Provider: "openai",
				BaseURL:  "https://api.openai.com/v1",
				Model:    "gpt-4o",
			},
			Admin: AdminSettings{
				Email:    "newchef@solar.de",
				Password: "NewSecretPassword456!",
			},
			System: SystemSettings{
				Port: 9000,
			},
		}
		body, _ := json.Marshal(newSettings)
		req := httptest.NewRequest(http.MethodPost, "/api/settings/import", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify updated .env
		envData, _ := os.ReadFile(filepath.Join(tmp, ".env"))
		c := string(envData)
		if !strings.Contains(c, "INITIAL_ADMIN_EMAIL=newchef@solar.de") {
			t.Errorf("expected updated email in .env")
		}
		if !strings.Contains(c, "AI_PROVIDER=openai") {
			t.Errorf("expected updated AI provider in .env")
		}
		if !strings.Contains(c, "PORT=9000") {
			t.Errorf("expected updated port in .env")
		}
	})

	t.Run("POST /api/settings/import via multipart file", func(t *testing.T) {
		fileSettings := ExportedSettings{
			AI: AISettings{
				Provider: "ollama",
				BaseURL:  "http://host.docker.internal:11434",
				Model:    "gemma4:12b",
			},
			Admin: AdminSettings{
				Email: "fileadmin@solar.de",
			},
			System: SystemSettings{
				Port: 8088,
			},
		}
		fileBytes, _ := json.Marshal(fileSettings)

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, _ := mw.CreateFormFile("settings", "openlocalcrm-settings.json")
		_, _ = part.Write(fileBytes)
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/settings/import", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		envData, _ := os.ReadFile(filepath.Join(tmp, ".env"))
		c := string(envData)
		if !strings.Contains(c, "INITIAL_ADMIN_EMAIL=fileadmin@solar.de") {
			t.Errorf("expected fileadmin@solar.de in .env")
		}
	})

	t.Run("Method Not Allowed checks", func(t *testing.T) {
		// POST to export -> 405
		req := httptest.NewRequest(http.MethodPost, "/api/settings/export", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 on POST export, got %d", rec.Code)
		}

		// GET to import -> 405
		req2 := httptest.NewRequest(http.MethodGet, "/api/settings/import", nil)
		rec2 := httptest.NewRecorder()
		server.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 on GET import, got %d", rec2.Code)
		}

		// GET to rebuild -> 405
		req3 := httptest.NewRequest(http.MethodGet, "/api/settings/rebuild", nil)
		rec3 := httptest.NewRecorder()
		server.ServeHTTP(rec3, req3)
		if rec3.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 on GET rebuild, got %d", rec3.Code)
		}
	})

	t.Run("POST /api/settings/import validation errors", func(t *testing.T) {
		// Invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/api/settings/import", bytes.NewReader([]byte("{invalid-json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
		}

		// Multipart missing file
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.Close()
		req2 := httptest.NewRequest(http.MethodPost, "/api/settings/import", &buf)
		req2.Header.Set("Content-Type", mw.FormDataContentType())
		rec2 := httptest.NewRecorder()
		server.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing multipart file, got %d", rec2.Code)
		}

		// Multipart invalid json
		var buf3 bytes.Buffer
		mw3 := multipart.NewWriter(&buf3)
		part3, _ := mw3.CreateFormFile("settings", "corrupt.json")
		_, _ = part3.Write([]byte("not-json"))
		_ = mw3.Close()
		req3 := httptest.NewRequest(http.MethodPost, "/api/settings/import", &buf3)
		req3.Header.Set("Content-Type", mw3.FormDataContentType())
		rec3 := httptest.NewRecorder()
		server.ServeHTTP(rec3, req3)
		if rec3.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for corrupt JSON in multipart, got %d", rec3.Code)
		}
	})

	t.Run("POST /api/settings/rebuild starts rebuild", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/settings/rebuild", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on rebuild, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&resp)
		if resp["success"] != true {
			t.Errorf("expected success true, got %v", resp["success"])
		}
		time.Sleep(200 * time.Millisecond)
	})
}
