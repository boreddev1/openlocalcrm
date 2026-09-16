package launcher

import (
	"os"
	"path/filepath"
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
