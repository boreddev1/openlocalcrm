package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEnvContent(t *testing.T) {
	cfg := SetupConfig{
		AdminEmail:    "admin@openlocalcrm.local",
		AdminPassword: "SuperSecretPassword123!",
		Port:          8080,
		IsDemoMode:    false,
		AIProvider:    "ollama",
		AIBaseURL:     "http://localhost:11434",
		AIModel:       "gemma2:12b",
	}

	content, err := GenerateEnvContent(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(content, "PORT=8080") {
		t.Errorf("expected PORT=8080 in env content")
	}
	if !strings.Contains(content, "INITIAL_ADMIN_PASSWORD=SuperSecretPassword123!") {
		t.Errorf("expected INITIAL_ADMIN_PASSWORD in env content")
	}
	if !strings.Contains(content, "OLLAMA_BASE_URL=http://host.docker.internal:11434") {
		t.Errorf("expected translated host.docker.internal URL in env content")
	}
}

func TestIsAlreadyInstalled(t *testing.T) {
	tmp := t.TempDir()
	if IsAlreadyInstalled(tmp) {
		t.Errorf("empty directory should not report already installed")
	}

	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte("PORT=80"), 0600)
	if !IsAlreadyInstalled(tmp) {
		t.Errorf("directory with .env should report already installed")
	}
}

func TestGenerateEnvContent_DatabaseSync(t *testing.T) {
	cfg := SetupConfig{
		Port:          8080,
		AdminEmail:    "admin@openlocalcrm.local",
		AdminPassword: "SecretPassword123!",
	}

	content, err := GenerateEnvContent(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(content, "APP_PORT=8080") {
		t.Errorf("expected APP_PORT=8080 in .env")
	}
	if !strings.Contains(content, "DB_HOST=db") {
		t.Errorf("expected DB_HOST=db to match docker service name")
	}
	if !strings.Contains(content, "@db:5432/openlocalcrm") {
		t.Errorf("expected @db:5432/openlocalcrm in DATABASE_URL")
	}
}

func TestGenerateEnvContent_LegacyDatabasePreserved(t *testing.T) {
	cfg := SetupConfig{
		Port:          8080,
		AdminEmail:    "admin@mavalio.local",
		AdminPassword: "SecretPassword123!",
	}
	existing := map[string]string{
		"DB_NAME":     "mavalio",
		"DB_PASSWORD": "legacy-db-pass",
	}

	content, err := GenerateEnvContentWithExisting(cfg, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(content, "@db:5432/mavalio") {
		t.Errorf("expected legacy @db:5432/mavalio in DATABASE_URL, got: %s", content)
	}
	if !strings.Contains(content, "DB_NAME=mavalio") {
		t.Errorf("expected legacy DB_NAME=mavalio, got: %s", content)
	}
}

func TestReadExistingConfig(t *testing.T) {
	tmp := t.TempDir()
	envContent := `PORT=9000
APP_PORT=9000
INITIAL_ADMIN_EMAIL=custom@domain.de
INITIAL_ADMIN_PASSWORD=MySecretPassword123!
DB_PASSWORD=keep-this-db-password
AI_PROVIDER=openai
OLLAMA_MODEL=gpt-4o-mini
`
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatalf("failed writing test .env: %v", err)
	}

	cfg, exists := ReadExistingConfig(tmp)
	if !exists {
		t.Fatalf("expected existing config to be detected")
	}
	if cfg.Port != 9000 {
		t.Errorf("expected Port 9000, got %d", cfg.Port)
	}
	if cfg.AdminEmail != "custom@domain.de" {
		t.Errorf("expected AdminEmail custom@domain.de, got %s", cfg.AdminEmail)
	}
	if cfg.AdminPassword != "MySecretPassword123!" {
		t.Errorf("expected AdminPassword MySecretPassword123!, got %s", cfg.AdminPassword)
	}
	if cfg.AIProvider != "openai" {
		t.Errorf("expected AIProvider openai, got %s", cfg.AIProvider)
	}
}

func TestGenerateEnvContentWithExisting_PreservesCredentials(t *testing.T) {
	tmp := t.TempDir()
	existing := map[string]string{
		"DB_PASSWORD":         "original-database-secret-42",
		"CONNECTOR_API_TOKEN": "original-connector-token-99",
		"INITIAL_ADMIN_EMAIL": "admin@domain.de",
	}

	newCfg := SetupConfig{
		Port:          8088,
		AdminEmail:    "admin@domain.de",
		AdminPassword: "NewAdminPassword123!",
		AIProvider:    "ollama",
	}

	content, err := GenerateEnvContentWithExisting(newCfg, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(content, "DB_PASSWORD=original-database-secret-42") {
		t.Errorf("expected DB_PASSWORD to be preserved from existing env")
	}
	if !strings.Contains(content, "CONNECTOR_API_TOKEN=original-connector-token-99") {
		t.Errorf("expected CONNECTOR_API_TOKEN to be preserved from existing env")
	}
	if !strings.Contains(content, "PORT=8088") {
		t.Errorf("expected new Port 8088 to be applied")
	}

	// Also test WriteConfigAndDirectories preserves credentials
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte("DB_PASSWORD=original-database-secret-42\nCONNECTOR_API_TOKEN=original-connector-token-99\n"), 0600)
	if err := WriteConfigAndDirectories(tmp, newCfg); err != nil {
		t.Fatalf("WriteConfigAndDirectories failed: %v", err)
	}

	saved, _ := os.ReadFile(filepath.Join(tmp, ".env"))
	if !strings.Contains(string(saved), "DB_PASSWORD=original-database-secret-42") {
		t.Errorf("expected WriteConfigAndDirectories to preserve DB_PASSWORD in written file")
	}
}

func TestWriteConfigAndDirectoriesWithEnv(t *testing.T) {
	tmp := t.TempDir()
	cfg := SetupConfig{
		Port:          80,
		AdminEmail:    "test@local.de",
		AdminPassword: "testpassword",
		AIProvider:    "ollama",
	}
	recovered := map[string]string{
		"DB_PASSWORD":         "recovered-db-pass-1234",
		"CONNECTOR_API_TOKEN": "recovered-token-5678",
	}

	if err := WriteConfigAndDirectoriesWithEnv(tmp, cfg, recovered); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, ".env"))
	if err != nil {
		t.Fatalf("failed reading written .env: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "DB_PASSWORD=recovered-db-pass-1234") {
		t.Errorf("expected recovered DB_PASSWORD in written .env")
	}
	if !strings.Contains(content, "CONNECTOR_API_TOKEN=recovered-token-5678") {
		t.Errorf("expected recovered CONNECTOR_API_TOKEN in written .env")
	}
	if !strings.Contains(content, "INITIAL_ADMIN_EMAIL=test@local.de") {
		t.Errorf("expected INITIAL_ADMIN_EMAIL in written .env")
	}
}

func TestGenerateEnvContent_Version(t *testing.T) {
	cfg := SetupConfig{
		Port:       80,
		AdminEmail: "admin@openlocalcrm.local",
		Version:    "v0.9",
	}

	content, err := GenerateEnvContent(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "OPENLOCALCRM_VERSION=v0.9") {
		t.Errorf("expected OPENLOCALCRM_VERSION=v0.9 in env content, got:\n%s", content)
	}

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(content), 0600)
	loaded, ok := ReadExistingConfig(tmp)
	if !ok || loaded.Version != "v0.9" {
		t.Errorf("expected ReadExistingConfig to read version v0.9, got %+v", loaded)
	}
}
