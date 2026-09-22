package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureComposeAndCaddyFiles(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureComposeAndCaddyFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	composeFile := filepath.Join(tmpDir, "docker-compose.yml")
	if _, err := os.Stat(composeFile); err != nil {
		t.Errorf("docker-compose.yml was not created in target dir")
	}

	caddyFile := filepath.Join(tmpDir, "Caddyfile")
	if _, err := os.Stat(caddyFile); err != nil {
		t.Errorf("Caddyfile was not created in target dir")
	}

	// Calling it a second time should not fail and should preserve existing files
	_ = os.WriteFile(composeFile, []byte("custom-compose"), 0644)
	err = EnsureComposeAndCaddyFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	content, _ := os.ReadFile(composeFile)
	if string(content) != "custom-compose" {
		t.Errorf("expected existing docker-compose.yml to not be overwritten")
	}
}

func TestPatchComposeEmbeddingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	content := "services:\n  crm-server:\n    environment:\n      - OLLAMA_MODEL=${OLLAMA_MODEL:-gemma4:12b}\n      - OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-qwen3-embedding:0.6b}\n      - AI_EMBEDDING_MODEL=${AI_EMBEDDING_MODEL:-}\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PatchComposeEmbeddingDefaults(path); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), ":-qwen3-embedding:0.6b}") {
		t.Errorf("expected qwen3 default removed, got:\n%s", got)
	}
	if !strings.Contains(string(got), "OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-}") {
		t.Errorf("expected empty default form, got:\n%s", got)
	}
	if !strings.Contains(string(got), "OLLAMA_MODEL=${OLLAMA_MODEL:-gemma4:12b}") {
		t.Errorf("chat model line must remain untouched, got:\n%s", got)
	}

	// Idempotent + fehlende Zeilen bleiben fehlend
	if err := PatchComposeEmbeddingDefaults(path); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(got) {
		t.Errorf("patch must be idempotent")
	}
	plain := "services:\n  crm-server:\n    environment:\n      - OLLAMA_MODEL=${OLLAMA_MODEL:-gemma4:12b}\n"
	if err := os.WriteFile(path, []byte(plain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := PatchComposeEmbeddingDefaults(path); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != plain {
		t.Errorf("missing embedding lines must stay missing, got:\n%s", unchanged)
	}
}

func TestDefaultDockerComposeHasEmptyEmbeddingDefaults(t *testing.T) {
	if strings.Contains(defaultDockerCompose, ":-qwen3-embedding:0.6b}") {
		t.Errorf("template must not hardcode qwen3 as compose default")
	}
	if !strings.Contains(defaultDockerCompose, "AI_EMBEDDING_MODEL=${AI_EMBEDDING_MODEL:-}") {
		t.Errorf("template must declare AI_EMBEDDING_MODEL with empty default")
	}
	if !strings.Contains(defaultDockerCompose, "OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-}") {
		t.Errorf("template must declare OLLAMA_EMBEDDING_MODEL with empty default")
	}
}
