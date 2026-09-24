package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AISettings contains AI configuration for local or cloud LLMs.
type AISettings struct {
	Provider       string `json:"provider"` // "ollama", "openai", "none"
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	HasAPIKey      bool   `json:"has_api_key,omitempty"`
}

// AdminSettings contains bootstrap credentials for the system administrator.
type AdminSettings struct {
	Email       string `json:"email"`
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"has_password,omitempty"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
}

// SystemSettings contains core network and connector parameters.
type SystemSettings struct {
	Port                 int    `json:"port"`
	DemoMode             bool   `json:"demo_mode"`
	ConnectorAPIToken    string `json:"connector_api_token,omitempty"`
	HasConnectorAPIToken bool   `json:"has_connector_api_token,omitempty"`
	Version              string `json:"version"`
}

// ExportedSettings is the complete portable configuration object for OpenLocalCRM.
type ExportedSettings struct {
	App        string         `json:"app"`
	Version    string         `json:"version"`
	ExportedAt string         `json:"exported_at"`
	AI         AISettings     `json:"ai"`
	Admin      AdminSettings  `json:"admin"`
	System     SystemSettings `json:"system"`
}

// RedactSecrets returns a sanitized copy of ExportedSettings without plaintext credentials.
func (s *ExportedSettings) RedactSecrets() *ExportedSettings {
	if s == nil {
		return nil
	}
	cp := *s
	cp.AI.HasAPIKey = s.AI.APIKey != "" || s.AI.HasAPIKey
	cp.AI.APIKey = ""
	cp.Admin.HasPassword = s.Admin.Password != "" || s.Admin.HasPassword
	cp.Admin.Password = ""
	cp.System.HasConnectorAPIToken = s.System.ConnectorAPIToken != "" || s.System.HasConnectorAPIToken
	cp.System.ConnectorAPIToken = ""
	return &cp
}

const DefaultVersion = "v1.0.3"

// ExportSettingsFromEnv extracts settings from .env in baseDir into an ExportedSettings struct.
func ExportSettingsFromEnv(baseDir string) (*ExportedSettings, error) {
	envPath := filepath.Join(baseDir, ".env")
	envMap := make(map[string]string)

	if data, err := os.ReadFile(envPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				envMap[key] = val
			}
		}
	}

	port := 80
	if pStr, ok := envMap["APP_PORT"]; ok {
		var p int
		if _, err := fmt.Sscanf(pStr, "%d", &p); err == nil && p > 0 {
			port = p
		}
	} else if pStr, ok := envMap["PORT"]; ok {
		var p int
		if _, err := fmt.Sscanf(pStr, "%d", &p); err == nil && p > 0 {
			port = p
		}
	}

	aiProvider := envMap["AI_PROVIDER"]
	if aiProvider == "" {
		aiProvider = "ollama"
	}

	aiBaseURL := envMap["OLLAMA_BASE_URL"]
	if aiBaseURL == "" {
		aiBaseURL = envMap["AI_BASE_URL"]
	}
	if aiBaseURL == "" {
		aiBaseURL = "http://host.docker.internal:11434"
	}

	aiModel := envMap["OLLAMA_MODEL"]
	if aiModel == "" {
		aiModel = "gemma4:12b"
	}

	adminEmail := envMap["INITIAL_ADMIN_EMAIL"]
	if adminEmail == "" {
		adminEmail = "admin@openlocalcrm.local"
	}

	adminFirst := envMap["INITIAL_ADMIN_FIRST_NAME"]
	if adminFirst == "" {
		adminFirst = "Admin"
	}

	adminLast := envMap["INITIAL_ADMIN_LAST_NAME"]
	if adminLast == "" {
		adminLast = "User"
	}

	ver := envMap["OPENLOCALCRM_VERSION"]
	if ver == "" {
		ver = DefaultVersion
	}

	return &ExportedSettings{
		App:        "OpenLocalCRM",
		Version:    ver,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		AI: AISettings{
			Provider:       aiProvider,
			BaseURL:        aiBaseURL,
			Model:          aiModel,
			EmbeddingModel: resolveEmbeddingModel(aiProvider, envMap["AI_EMBEDDING_MODEL"], envMap["AI_EMBEDDING_MODEL"], envMap["OLLAMA_EMBEDDING_MODEL"]),
			HasAPIKey:      envMap["AI_API_KEY"] != "",
		},
		Admin: AdminSettings{
			Email:       adminEmail,
			HasPassword: envMap["INITIAL_ADMIN_PASSWORD"] != "",
			FirstName:   adminFirst,
			LastName:    adminLast,
		},
		System: SystemSettings{
			Port:                 port,
			DemoMode:             strings.EqualFold(envMap["DEMO_MODE"], "true"),
			HasConnectorAPIToken: envMap["CONNECTOR_API_TOKEN"] != "",
			Version:              ver,
		},
	}, nil
}

// ValidateSettingsValues rejects any setting containing newline control characters to prevent .env injection.
func ValidateSettingsValues(s *ExportedSettings) error {
	if s == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	check := func(name, val string) error {
		if strings.ContainsAny(val, "\r\n") {
			return fmt.Errorf("field %q contains invalid control characters (newlines)", name)
		}
		return nil
	}
	if err := check("ai.provider", s.AI.Provider); err != nil {
		return err
	}
	if err := check("ai.base_url", s.AI.BaseURL); err != nil {
		return err
	}
	if err := check("ai.model", s.AI.Model); err != nil {
		return err
	}
	if err := check("ai.embedding_model", s.AI.EmbeddingModel); err != nil {
		return err
	}
	if err := check("ai.api_key", s.AI.APIKey); err != nil {
		return err
	}
	if err := check("admin.email", s.Admin.Email); err != nil {
		return err
	}
	if err := check("admin.password", s.Admin.Password); err != nil {
		return err
	}
	if err := check("admin.first_name", s.Admin.FirstName); err != nil {
		return err
	}
	if err := check("admin.last_name", s.Admin.LastName); err != nil {
		return err
	}
	if err := check("system.connector_api_token", s.System.ConnectorAPIToken); err != nil {
		return err
	}
	if err := check("system.version", s.System.Version); err != nil {
		return err
	}
	return nil
}

// ImportSettingsToEnv merges the settings into baseDir/.env.
func ImportSettingsToEnv(baseDir string, s *ExportedSettings, keepExistingSecrets bool) error {
	if s == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	if err := ValidateSettingsValues(s); err != nil {
		return err
	}

	envPath := filepath.Join(baseDir, ".env")
	existing := make(map[string]string)

	if data, err := os.ReadFile(envPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				existing[key] = val
			}
		}
	}

	cfg := SetupConfig{
		Port:             s.System.Port,
		AdminEmail:       s.Admin.Email,
		AdminPassword:    s.Admin.Password,
		IsDemoMode:       s.System.DemoMode,
		AIProvider:       s.AI.Provider,
		AIBaseURL:        s.AI.BaseURL,
		AIModel:          s.AI.Model,
		AIEmbeddingModel: s.AI.EmbeddingModel,
		AIAPIKey:         s.AI.APIKey,
		Version:          s.System.Version,
	}

	if cfg.Port <= 0 {
		cfg.Port = 80
	}
	if cfg.AdminEmail == "" {
		cfg.AdminEmail = "admin@openlocalcrm.local"
	}
	if cfg.AIProvider == "" {
		cfg.AIProvider = "ollama"
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = "http://host.docker.internal:11434"
	}
	if cfg.AIModel == "" {
		cfg.AIModel = "gemma4:12b"
	}
	if cfg.Version == "" {
		cfg.Version = DefaultVersion
	}

	// Keep existing secrets if import did not specify one
	if cfg.AdminPassword == "" && keepExistingSecrets && existing["INITIAL_ADMIN_PASSWORD"] != "" {
		cfg.AdminPassword = existing["INITIAL_ADMIN_PASSWORD"]
	}
	if cfg.AIAPIKey == "" && keepExistingSecrets && existing["AI_API_KEY"] != "" {
		cfg.AIAPIKey = existing["AI_API_KEY"]
	}
	if s.System.ConnectorAPIToken != "" {
		existing["CONNECTOR_API_TOKEN"] = s.System.ConnectorAPIToken
	}
	if s.Admin.FirstName != "" {
		existing["INITIAL_ADMIN_FIRST_NAME"] = s.Admin.FirstName
	}
	if s.Admin.LastName != "" {
		existing["INITIAL_ADMIN_LAST_NAME"] = s.Admin.LastName
	}

	content, err := GenerateEnvContentWithExisting(cfg, existing)
	if err != nil {
		return fmt.Errorf("failed generating env content: %w", err)
	}

	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed writing .env file: %w", err)
	}

	return nil
}

// SaveSettingsToFile serializes the settings to a formatted JSON file.
func SaveSettingsToFile(filePath string, s *ExportedSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed marshalling settings: %w", err)
	}
	return os.WriteFile(filePath, data, 0600)
}

// LoadSettingsFromFile reads and unmarshals an ExportedSettings JSON file.
func LoadSettingsFromFile(filePath string) (*ExportedSettings, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading settings file: %w", err)
	}
	var s ExportedSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid settings json: %w", err)
	}
	return &s, nil
}
