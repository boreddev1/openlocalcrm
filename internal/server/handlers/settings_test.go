package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/launcher"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestSettingsHandler_ExportImport(t *testing.T) {
	tmp := t.TempDir()
	envContent := `PORT=8080
INITIAL_ADMIN_EMAIL=admin@system.de
AI_PROVIDER=ollama
OLLAMA_MODEL=gemma4:12b
`
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(envContent), 0600)

	h := handlers.NewSettingsHandler(tmp)

	adminCtx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		Email: "admin@system.de",
		Role:  "ADMIN",
	})
	userCtx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		Email: "user@system.de",
		Role:  "BENUTZER",
	})

	t.Run("ExportSettings forbidden for non-admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/export", nil).WithContext(userCtx)
		rec := httptest.NewRecorder()

		h.ExportSettings(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ExportSettings success for admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/export", nil).WithContext(adminCtx)
		rec := httptest.NewRecorder()

		h.ExportSettings(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var exported launcher.ExportedSettings
		err := json.NewDecoder(rec.Body).Decode(&exported)
		require.NoError(t, err)
		require.Equal(t, "ollama", exported.AI.Provider)
		require.Equal(t, "gemma4:12b", exported.AI.Model)
		require.Equal(t, "admin@system.de", exported.Admin.Email)
		require.Equal(t, 8080, exported.System.Port)
	})

	t.Run("ImportSettings forbidden for non-admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader([]byte("{}"))).WithContext(userCtx)
		rec := httptest.NewRecorder()

		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ImportSettings success for admin via JSON", func(t *testing.T) {
		newSettings := launcher.ExportedSettings{
			AI: launcher.AISettings{
				Provider: "openai",
				BaseURL:  "https://api.openai.com/v1",
				Model:    "gpt-4o",
			},
			Admin: launcher.AdminSettings{
				Email: "new-admin@solar.de",
			},
			System: launcher.SystemSettings{
				Port: 8888,
			},
		}
		body, _ := json.Marshal(newSettings)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader(body)).WithContext(adminCtx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		// Verify updated .env
		data, _ := os.ReadFile(filepath.Join(tmp, ".env"))
		c := string(data)
		require.Contains(t, c, "INITIAL_ADMIN_EMAIL=new-admin@solar.de")
		require.Contains(t, c, "AI_PROVIDER=openai")
		require.Contains(t, c, "PORT=8888")
	})

	t.Run("ImportSettings invalid JSON in body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader([]byte("{corrupted-json"))).WithContext(adminCtx)
		rec := httptest.NewRecorder()
		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ImportSettings via multipart form", func(t *testing.T) {
		fileSettings := launcher.ExportedSettings{
			AI: launcher.AISettings{
				Provider: "ollama",
				BaseURL:  "http://host.docker.internal:11434",
				Model:    "gemma4:12b",
			},
			Admin: launcher.AdminSettings{
				Email: "multipart-admin@solar.de",
			},
			System: launcher.SystemSettings{
				Port: 8089,
			},
		}
		fileBytes, _ := json.Marshal(fileSettings)

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, _ := mw.CreateFormFile("settings", "openlocalcrm-settings.json")
		_, _ = part.Write(fileBytes)
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", &buf).WithContext(adminCtx)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()

		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		data, _ := os.ReadFile(filepath.Join(tmp, ".env"))
		require.Contains(t, string(data), "INITIAL_ADMIN_EMAIL=multipart-admin@solar.de")
	})

	t.Run("ExportSettings never outputs plaintext secrets (F-31)", func(t *testing.T) {
		secEnv := `PORT=8080
INITIAL_ADMIN_EMAIL=admin@system.de
INITIAL_ADMIN_PASSWORD=SuperSecretAdminPassword123!
AI_API_KEY=sk-proj-ultra-secret-key-456
CONNECTOR_API_TOKEN=super-secret-connector-token-789
`
		_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(secEnv), 0600)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/export", nil).WithContext(adminCtx)
		rec := httptest.NewRecorder()

		h.ExportSettings(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		rawBody := rec.Body.String()
		require.NotContains(t, rawBody, "SuperSecretAdminPassword123!")
		require.NotContains(t, rawBody, "sk-proj-ultra-secret-key-456")
		require.NotContains(t, rawBody, "super-secret-connector-token-789")

		var exported launcher.ExportedSettings
		err := json.Unmarshal([]byte(rawBody), &exported)
		require.NoError(t, err)
		require.Empty(t, exported.Admin.Password)
		require.True(t, exported.Admin.HasPassword)
		require.Empty(t, exported.AI.APIKey)
		require.True(t, exported.AI.HasAPIKey)
		require.Empty(t, exported.System.ConnectorAPIToken)
		require.True(t, exported.System.HasConnectorAPIToken)
	})

	t.Run("ImportSettings rejects newlines to prevent .env injection (F-33)", func(t *testing.T) {
		injectedSettings := launcher.ExportedSettings{
			Admin: launcher.AdminSettings{
				Email: "admin@system.de\nINJECTED_VAR=malicious_payload",
			},
		}
		body, _ := json.Marshal(injectedSettings)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader(body)).WithContext(adminCtx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "invalid control characters")
	})

	t.Run("ImportSettings does not echo secrets in response (F-32)", func(t *testing.T) {
		settingsWithSecrets := launcher.ExportedSettings{
			Admin: launcher.AdminSettings{
				Email:    "new@system.de",
				Password: "NewPlaintextPassword999!",
			},
			AI: launcher.AISettings{
				APIKey: "sk-another-secret-key-111",
			},
			System: launcher.SystemSettings{
				ConnectorAPIToken: "conn-secret-token-222",
			},
		}
		body, _ := json.Marshal(settingsWithSecrets)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/import", bytes.NewReader(body)).WithContext(adminCtx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		h.ImportSettings(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		rawBody := rec.Body.String()
		require.NotContains(t, rawBody, "NewPlaintextPassword999!")
		require.NotContains(t, rawBody, "sk-another-secret-key-111")
		require.NotContains(t, rawBody, "conn-secret-token-222")
	})
}
