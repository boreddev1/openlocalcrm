package launcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SetupConfig struct {
	AdminEmail    string `json:"admin_email"`
	AdminPassword string `json:"admin_password"`
	Port          int    `json:"port"`
	IsDemoMode    bool   `json:"is_demo_mode"`
	AIProvider    string `json:"ai_provider"`
	AIBaseURL     string `json:"ai_base_url"`
	AIAPIKey      string `json:"ai_api_key"`
	AIModel       string `json:"ai_model"`
	Version       string `json:"version"`
}

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IsAlreadyInstalled checks if a valid .env configuration exists in baseDir
func IsAlreadyInstalled(baseDir string) bool {
	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		return true
	}
	return false
}

// ReadExistingEnvMap parses key-value pairs from an existing .env file
func ReadExistingEnvMap(baseDir string) map[string]string {
	envPath := filepath.Join(baseDir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return make(map[string]string)
	}

	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			result[k] = v
		}
	}
	return result
}

// ReadExistingConfig loads existing configuration from .env into a SetupConfig
func ReadExistingConfig(baseDir string) (*SetupConfig, bool) {
	envMap := ReadExistingEnvMap(baseDir)
	if len(envMap) == 0 {
		return nil, false
	}

	cfg := &SetupConfig{
		AdminEmail:    envMap["INITIAL_ADMIN_EMAIL"],
		AdminPassword: envMap["INITIAL_ADMIN_PASSWORD"],
		Port:          80,
		IsDemoMode:    strings.EqualFold(envMap["DEMO_MODE"], "true"),
		AIProvider:    envMap["AI_PROVIDER"],
		AIBaseURL:     envMap["AI_BASE_URL"],
		AIAPIKey:      envMap["AI_API_KEY"],
		AIModel:       envMap["OLLAMA_MODEL"],
		Version:       envMap["OPENLOCALCRM_VERSION"],
	}

	if cfg.Version == "" {
		cfg.Version = envMap["CRM_VERSION"]
	}
	if cfg.Version == "" {
		cfg.Version = "v0.9"
	}

	if cfg.AdminEmail == "" {
		cfg.AdminEmail = "admin@openlocalcrm.local"
	}
	if cfg.AIProvider == "" {
		cfg.AIProvider = "ollama"
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = envMap["OLLAMA_BASE_URL"]
	}

	if pStr, ok := envMap["APP_PORT"]; ok {
		var p int
		if _, err := fmt.Sscanf(pStr, "%d", &p); err == nil && p > 0 {
			cfg.Port = p
		}
	} else if pStr, ok := envMap["PORT"]; ok {
		var p int
		if _, err := fmt.Sscanf(pStr, "%d", &p); err == nil && p > 0 {
			cfg.Port = p
		}
	}

	return cfg, true
}

// DetectExistingInstallation checks whether OpenLocalCRM is already installed or running
func DetectExistingInstallation(ctx context.Context, baseDir string, engine *Engine) (bool, string) {
	// 1. Local .env file
	if IsAlreadyInstalled(baseDir) {
		return true, "Lokale Konfigurationsdatei (.env) gefunden"
	}

	// 2. Check if Docker containers already exist on host and recover their configuration
	if engine != nil {
		if recovered, err := engine.RecoverEnvFromDocker(ctx); err == nil && len(recovered) > 0 {
			if recovered["DB_PASSWORD"] != "" || recovered["DATABASE_URL"] != "" {
				port := 80
				if pStr, ok := recovered["APP_PORT"]; ok {
					_, _ = fmt.Sscanf(pStr, "%d", &port)
				} else if pStr, ok := recovered["PORT"]; ok {
					_, _ = fmt.Sscanf(pStr, "%d", &port)
				}

				cfg := SetupConfig{
					AdminEmail:    recovered["INITIAL_ADMIN_EMAIL"],
					AdminPassword: recovered["INITIAL_ADMIN_PASSWORD"],
					Port:          port,
					IsDemoMode:    strings.EqualFold(recovered["DEMO_MODE"], "true"),
					AIProvider:    recovered["AI_PROVIDER"],
					AIBaseURL:     recovered["OLLAMA_BASE_URL"],
					AIAPIKey:      recovered["AI_API_KEY"],
					AIModel:       recovered["OLLAMA_MODEL"],
					Version:       recovered["OPENLOCALCRM_VERSION"],
				}
				if cfg.AdminEmail == "" {
					cfg.AdminEmail = "admin@openlocalcrm.local"
				}
				if cfg.AIProvider == "" {
					cfg.AIProvider = "ollama"
				}
				_ = WriteConfigAndDirectoriesWithEnv(baseDir, cfg, recovered)
				return true, "Bestehende Docker-Container (crm-*) erkannt und Konfiguration automatisch wiederhergestellt"
			}
		}

		if containers, err := engine.GetContainers(ctx); err == nil && len(containers) > 0 {
			for _, c := range containers {
				if strings.Contains(c.Name, "crm") || strings.Contains(c.Name, "openlocalcrm") || strings.Contains(c.Name, "mavalio") || strings.Contains(c.Service, "server") || strings.Contains(c.Service, "db") || strings.Contains(c.Service, "caddy") {
					if !IsAlreadyInstalled(baseDir) {
						defaultCfg := SetupConfig{
							AdminEmail: "admin@openlocalcrm.local",
							Port:       80,
							AIProvider: "ollama",
						}
						_ = WriteConfigAndDirectories(baseDir, defaultCfg)
					}
					return true, fmt.Sprintf("Bestehende Docker-Dienste (%s: %s) erkannt", c.Service, c.State)
				}
			}
		}
	}

	// 3. HTTP Health check on localhost
	client := &http.Client{Timeout: 800 * time.Millisecond}
	for _, url := range []string{"http://127.0.0.1/api/v1/health", "http://127.0.0.1:8080/api/v1/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true, fmt.Sprintf("Laufende OpenLocalCRM Instanz auf %s aktiv", url)
				}
			}
		}
	}

	return false, ""
}

func GenerateEnvContent(cfg SetupConfig) (string, error) {
	return GenerateEnvContentWithExisting(cfg, nil)
}

func GenerateEnvContentWithExisting(cfg SetupConfig, existing map[string]string) (string, error) {
	if cfg.Port <= 0 {
		cfg.Port = 80
	}
	if cfg.AdminEmail == "" {
		cfg.AdminEmail = "admin@openlocalcrm.local"
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = randomHex(12)
	}

	version := cfg.Version
	if version == "" {
		if existing != nil && existing["OPENLOCALCRM_VERSION"] != "" {
			version = existing["OPENLOCALCRM_VERSION"]
		} else {
			version = "v1.0.0"
		}
	}

	var serverImage, workerImage string
	if IsGHCRImageSupported(version) {
		imageTag := version
		if imageTag == "" || imageTag == "main" || imageTag == "master" {
			imageTag = "latest"
		}
		serverImage = fmt.Sprintf("ghcr.io/boreddev1/openlocalcrm/server:%s", imageTag)
		workerImage = fmt.Sprintf("ghcr.io/boreddev1/openlocalcrm/worker:%s", imageTag)
	}

	dbPassword := randomHex(16)
	if existing != nil && existing["DB_PASSWORD"] != "" {
		dbPassword = existing["DB_PASSWORD"]
	}

	dbName := "openlocalcrm"
	if existing != nil && existing["DB_NAME"] != "" {
		dbName = existing["DB_NAME"]
	}

	connectorToken := randomHex(24)
	if existing != nil && existing["CONNECTOR_API_TOKEN"] != "" {
		connectorToken = existing["CONNECTOR_API_TOKEN"]
	}

	aiBaseURL := cfg.AIBaseURL
	if aiBaseURL != "" {
		aiBaseURL = TranslateHostForDocker(aiBaseURL)
	} else if cfg.AIProvider == "ollama" {
		aiBaseURL = "http://host.docker.internal:11434"
	}

	lines := []string{
		"# ==============================================================================",
		"# OpenLocalCRM - Auto-generated Production Configuration",
		"# ==============================================================================",
		"DOMAIN=localhost",
		fmt.Sprintf("PORT=%d", cfg.Port),
		fmt.Sprintf("APP_PORT=%d", cfg.Port),
		fmt.Sprintf("OPENLOCALCRM_VERSION=%s", version),
		fmt.Sprintf("CRM_SERVER_IMAGE=%s", serverImage),
		fmt.Sprintf("CRM_WORKER_IMAGE=%s", workerImage),
		"LOG_LEVEL=info",
		"",
		"# Administrator Account",
		fmt.Sprintf("INITIAL_ADMIN_EMAIL=%s", cfg.AdminEmail),
		fmt.Sprintf("INITIAL_ADMIN_PASSWORD=%s", cfg.AdminPassword),
		"",
		"# Database (PostgreSQL 16)",
		"DB_HOST=db",
		"DB_PORT=5432",
		fmt.Sprintf("DB_NAME=%s", dbName),
		"DB_USER=postgres",
		fmt.Sprintf("DB_PASSWORD=%s", dbPassword),
		fmt.Sprintf("DATABASE_URL=postgres://postgres:%s@db:5432/%s?sslmode=disable", dbPassword, dbName),
		"",
		"# Authentication & Keys",
		"JWT_SECRET_KEY_PATH=/storage/keys/ed25519.key",
		"STORAGE_PATH=/storage",
		"",
		"# Connectors & Webhooks",
		fmt.Sprintf("CONNECTOR_API_TOKEN=%s", connectorToken),
		"",
		"# AI Gateway",
		fmt.Sprintf("AI_PROVIDER=%s", cfg.AIProvider),
		fmt.Sprintf("OLLAMA_BASE_URL=%s", aiBaseURL),
		fmt.Sprintf("OLLAMA_MODEL=%s", cfg.AIModel),
		fmt.Sprintf("AI_API_KEY=%s", cfg.AIAPIKey),
		fmt.Sprintf("AI_BASE_URL=%s", aiBaseURL),
		"",
	}

	if cfg.IsDemoMode {
		lines = append(lines, "DEMO_MODE=true", "")
	}

	return strings.Join(lines, "\n"), nil
}

func WriteConfigAndDirectories(baseDir string, cfg SetupConfig) error {
	existing := ReadExistingEnvMap(baseDir)
	return WriteConfigAndDirectoriesWithEnv(baseDir, cfg, existing)
}

func WriteConfigAndDirectoriesWithEnv(baseDir string, cfg SetupConfig, existing map[string]string) error {
	content, err := GenerateEnvContentWithExisting(cfg, existing)
	if err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(baseDir, "data", "postgres"),
		filepath.Join(baseDir, "data", "storage", "keys"),
		filepath.Join(baseDir, "backups"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed creating directory %s: %w", d, err)
		}
	}

	envPath := filepath.Join(baseDir, ".env")
	return os.WriteFile(envPath, []byte(content), 0600)
}

// IsGHCRImageSupported returns true if the selected release version is >= v1.0 or edge (main/master/latest).
// For older legacy releases (< v1.0, e.g. v0.9), it returns false so that images are built locally from source.
func IsGHCRImageSupported(version string) bool {
	v := strings.TrimSpace(strings.ToLower(version))
	if v == "" || v == "main" || v == "master" || v == "latest" || v == "edge" {
		return true
	}
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) > 0 {
		major, err := strconv.Atoi(parts[0])
		if err == nil {
			return major >= 1
		}
	}
	return false
}
