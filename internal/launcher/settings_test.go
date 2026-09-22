package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettings_ExportAndImportRoundtrip(t *testing.T) {
	tmp := t.TempDir()

	// 1. Initial setup of .env
	initialEnv := `PORT=8080
INITIAL_ADMIN_EMAIL=chef@solarbetrieb.de
INITIAL_ADMIN_PASSWORD=GeheimesPasswort123!
INITIAL_ADMIN_FIRST_NAME=Klaus
INITIAL_ADMIN_LAST_NAME=Sonne
AI_PROVIDER=ollama
OLLAMA_BASE_URL=http://host.docker.internal:11434
OLLAMA_MODEL=gemma4:12b
AI_API_KEY=test-api-key
CONNECTOR_API_TOKEN=custom-token-xyz
DEMO_MODE=false
OPENLOCALCRM_VERSION=v1.0.2
`
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte(initialEnv), 0600); err != nil {
		t.Fatalf("failed setting up test env: %v", err)
	}

	// 2. Export settings
	exported, err := ExportSettingsFromEnv(tmp)
	if err != nil {
		t.Fatalf("failed exporting settings: %v", err)
	}

	if exported.AI.Provider != "ollama" {
		t.Errorf("expected AI provider ollama, got %s", exported.AI.Provider)
	}
	if exported.AI.Model != "gemma4:12b" {
		t.Errorf("expected AI model gemma4:12b, got %s", exported.AI.Model)
	}
	if exported.Admin.Email != "chef@solarbetrieb.de" {
		t.Errorf("expected Admin email chef@solarbetrieb.de, got %s", exported.Admin.Email)
	}
	if exported.Admin.FirstName != "Klaus" {
		t.Errorf("expected Klaus, got %s", exported.Admin.FirstName)
	}
	if exported.System.Port != 8080 {
		t.Errorf("expected port 8080, got %d", exported.System.Port)
	}
	if exported.System.ConnectorAPIToken != "" {
		t.Errorf("expected redacted connector token, got %s", exported.System.ConnectorAPIToken)
	}
	if !exported.System.HasConnectorAPIToken {
		t.Errorf("expected HasConnectorAPIToken true")
	}
	if exported.Admin.Password != "" {
		t.Errorf("expected redacted admin password, got %s", exported.Admin.Password)
	}
	if !exported.Admin.HasPassword {
		t.Errorf("expected HasPassword true")
	}

	// 3. Save to settings.json and load back
	settingsFile := filepath.Join(tmp, "openlocalcrm-settings.json")
	if err := SaveSettingsToFile(settingsFile, exported); err != nil {
		t.Fatalf("failed saving settings file: %v", err)
	}

	loaded, err := LoadSettingsFromFile(settingsFile)
	if err != nil {
		t.Fatalf("failed loading settings file: %v", err)
	}
	if loaded.Admin.Email != exported.Admin.Email {
		t.Errorf("loaded email mismatch: %s vs %s", loaded.Admin.Email, exported.Admin.Email)
	}

	// 4. Simulate clean rebuild directory with existing .env secrets
	rebuildDir := t.TempDir()
	existingEnv := `INITIAL_ADMIN_PASSWORD=GeheimesPasswort123!
CONNECTOR_API_TOKEN=custom-token-xyz
`
	_ = os.WriteFile(filepath.Join(rebuildDir, ".env"), []byte(existingEnv), 0600)

	if err := ImportSettingsToEnv(rebuildDir, loaded, true); err != nil {
		t.Fatalf("failed importing settings to rebuild dir: %v", err)
	}

	// 5. Verify the newly generated .env
	rebuiltData, err := os.ReadFile(filepath.Join(rebuildDir, ".env"))
	if err != nil {
		t.Fatalf("failed reading rebuilt .env: %v", err)
	}
	content := string(rebuiltData)

	if !strings.Contains(content, "PORT=8080") {
		t.Errorf("expected PORT=8080 in rebuilt .env")
	}
	if !strings.Contains(content, "INITIAL_ADMIN_EMAIL=chef@solarbetrieb.de") {
		t.Errorf("expected admin email in rebuilt .env")
	}
	if !strings.Contains(content, "INITIAL_ADMIN_PASSWORD=GeheimesPasswort123!") {
		t.Errorf("expected admin password in rebuilt .env")
	}
	if !strings.Contains(content, "CONNECTOR_API_TOKEN=custom-token-xyz") {
		t.Errorf("expected connector token in rebuilt .env")
	}
	if !strings.Contains(content, "AI_PROVIDER=ollama") {
		t.Errorf("expected AI_PROVIDER=ollama in rebuilt .env")
	}
	if !strings.Contains(content, "OLLAMA_MODEL=gemma4:12b") {
		t.Errorf("expected OLLAMA_MODEL=gemma4:12b in rebuilt .env")
	}
}

func TestSettings_ImportPreservesExistingPasswordWhenEmpty(t *testing.T) {
	tmp := t.TempDir()
	initialEnv := `INITIAL_ADMIN_EMAIL=existing@admin.de
INITIAL_ADMIN_PASSWORD=PreserveMe1234!
PORT=80
`
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(initialEnv), 0600)

	imported := &ExportedSettings{
		AI: AISettings{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
			Model:    "gpt-4o-mini",
		},
		Admin: AdminSettings{
			Email:    "new@admin.de",
			Password: "", // Empty password in import
		},
		System: SystemSettings{
			Port:    8080,
			Version: "v1.0.2",
		},
	}

	if err := ImportSettingsToEnv(tmp, imported, true); err != nil {
		t.Fatalf("failed importing: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, ".env"))
	content := string(data)

	if !strings.Contains(content, "INITIAL_ADMIN_PASSWORD=PreserveMe1234!") {
		t.Errorf("expected existing password to be preserved, got:\n%s", content)
	}
	if !strings.Contains(content, "INITIAL_ADMIN_EMAIL=new@admin.de") {
		t.Errorf("expected updated email in .env")
	}
	if !strings.Contains(content, "AI_PROVIDER=openai") {
		t.Errorf("expected AI_PROVIDER=openai in .env")
	}
}

func TestSettingsExportImportEmbeddingModel(t *testing.T) {
	dir := t.TempDir()
	env := "AI_PROVIDER=ollama\nAI_MODEL=gemma4:12b\nOLLAMA_EMBEDDING_MODEL=qwen3-embedding:0.6b\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	exported, err := ExportSettingsFromEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if exported.AI.EmbeddingModel != "qwen3-embedding:0.6b" {
		t.Fatalf("expected alias-resolved export value, got %q", exported.AI.EmbeddingModel)
	}

	imported := &ExportedSettings{AI: AISettings{Provider: "ollama", Model: "gemma4:12b", EmbeddingModel: "mistral-embed"}}
	if err := ValidateSettingsValues(imported); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := ValidateSettingsValues(&ExportedSettings{AI: AISettings{EmbeddingModel: "bad\nmodel"}}); err == nil {
		t.Fatalf("expected newline injection to be rejected")
	}

	outDir := t.TempDir()
	if err := ImportSettingsToEnv(outDir, imported, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "AI_EMBEDDING_MODEL=mistral-embed") {
		t.Fatalf("expected import to write AI_EMBEDDING_MODEL, got:\n%s", content)
	}
}
