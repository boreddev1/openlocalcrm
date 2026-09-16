package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectBaseDir_CustomDir(t *testing.T) {
	tempDir := t.TempDir()
	got := ResolveProjectBaseDir(tempDir)
	if got != tempDir {
		t.Errorf("expected %s, got %s", tempDir, got)
	}
}

func TestResolveProjectBaseDir_EnvVar(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("OPENLOCALCRM_DIR", tempDir)
	got := ResolveProjectBaseDir("")
	if got != tempDir {
		t.Errorf("expected %s, got %s", tempDir, got)
	}
}

func TestResolveProjectBaseDir_CurrentDirWithCompose(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "docker-compose.yml"), []byte("services:"), 0644)
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	got := ResolveProjectBaseDir("")
	if got != "." && got != tempDir {
		t.Errorf("expected . or %s, got %s", tempDir, got)
	}
}
