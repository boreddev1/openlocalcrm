package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/launcher"
)

type SettingsHandler struct {
	baseDir string
	audit   *audit.Service
}

func NewSettingsHandler(baseDir ...string) *SettingsHandler {
	dir := "."
	if len(baseDir) > 0 && baseDir[0] != "" {
		dir = baseDir[0]
	}
	return &SettingsHandler{baseDir: dir}
}

func (h *SettingsHandler) WithAudit(auditSvc *audit.Service) *SettingsHandler {
	h.audit = auditSvc
	return h
}

// ExportSettings exports current system configuration (AI settings, Admin, Port, Connector token) without full database dump.
func (h *SettingsHandler) ExportSettings(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Systemeinstellungen exportieren"}`, http.StatusForbidden)
		return
	}

	// Try reading from .env in baseDir first, fallback to os.Getenv
	exported, err := launcher.ExportSettingsFromEnv(h.baseDir)
	if err != nil || exported.AI.Provider == "" {
		port := 8080
		if pStr := os.Getenv("PORT"); pStr != "" {
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
				port = p
			}
		}

		adminEmail := os.Getenv("INITIAL_ADMIN_EMAIL")
		if adminEmail == "" {
			adminEmail = claims.Email
		}

		aiProv := os.Getenv("AI_PROVIDER")
		if aiProv == "" {
			aiProv = "ollama"
		}

		aiBaseURL := os.Getenv("OLLAMA_BASE_URL")
		if aiBaseURL == "" {
			aiBaseURL = os.Getenv("AI_BASE_URL")
		}
		if aiBaseURL == "" {
			aiBaseURL = "http://host.docker.internal:11434"
		}

		aiModel := os.Getenv("OLLAMA_MODEL")
		if aiModel == "" {
			aiModel = "gemma2:12b"
		}

		exported = &launcher.ExportedSettings{
			App:        "OpenLocalCRM",
			Version:    launcher.DefaultVersion,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
			AI: launcher.AISettings{
				Provider:  aiProv,
				BaseURL:   aiBaseURL,
				Model:     aiModel,
				HasAPIKey: os.Getenv("AI_API_KEY") != "",
			},
			Admin: launcher.AdminSettings{
				Email:       adminEmail,
				HasPassword: os.Getenv("INITIAL_ADMIN_PASSWORD") != "",
			},
			System: launcher.SystemSettings{
				Port:                 port,
				DemoMode:             strings.EqualFold(os.Getenv("DEMO_MODE"), "true"),
				HasConnectorAPIToken: os.Getenv("CONNECTOR_API_TOKEN") != "",
				Version:              launcher.DefaultVersion,
			},
		}
	}

	if h.audit != nil {
		actingUserID := pgtype.UUID{Bytes: claims.UserID, Valid: true}
		_ = h.audit.Log(r.Context(), actingUserID, "SYSTEM", pgtype.UUID{}, "SETTINGS_EXPORTED", map[string]any{
			"action": "export",
		}, auth.ClientIP(r), r.UserAgent())
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="openlocalcrm-settings.json"`)
	_ = json.NewEncoder(w).Encode(exported.RedactSecrets())
}

// ImportSettings imports settings from a JSON payload and updates the configuration.
func (h *SettingsHandler) ImportSettings(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Systemeinstellungen importieren"}`, http.StatusForbidden)
		return
	}

	var settings launcher.ExportedSettings
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart/form-data") {
		file, _, err := r.FormFile("settings")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "missing_file", "Einstellungsdatei erforderlich")
			return
		}
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&settings); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json", "Ungültiges JSON-Format")
			return
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_body", "Ungültiger Anfrageinhalt")
			return
		}
	}

	if err := launcher.ImportSettingsToEnv(h.baseDir, &settings, true); err != nil {
		if strings.Contains(err.Error(), "invalid control characters") || strings.Contains(err.Error(), "cannot be nil") {
			writeJSONError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "import_failed", "Einstellungen konnten nicht importiert werden")
		return
	}

	if h.audit != nil {
		actingUserID := pgtype.UUID{Bytes: claims.UserID, Valid: true}
		_ = h.audit.Log(r.Context(), actingUserID, "SYSTEM", pgtype.UUID{}, "SETTINGS_IMPORTED", map[string]any{
			"action": "import",
		}, auth.ClientIP(r), r.UserAgent())
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"message":  "Einstellungen erfolgreich importiert. Bitte starten Sie die Container bei Bedarf neu.",
		"settings": settings.RedactSecrets(),
	})
}
