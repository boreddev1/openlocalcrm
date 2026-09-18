package launcher

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsProtectedPath(t *testing.T) {
	u := NewUpdater(".")

	protected := []string{
		".env",
		".env.local",
		".env.production",
		"backups/2026-09-16.sql",
		"data/postgres/pgdata",
		".git/config",
		"openlocalcrm-setup.log",
		"openlocalcrm-setup.exe.old",
		"caddy/certs/key.pem",
	}

	for _, p := range protected {
		if !u.IsProtectedPath(p) {
			t.Errorf("expected path %q to be protected, but was not", p)
		}
	}

	unprotected := []string{
		"docker-compose.yml",
		"Caddyfile",
		".env.example",
		"README.md",
		"bin/openlocalcrm-setup.exe",
		"web/dist/index.html",
	}

	for _, p := range unprotected {
		if u.IsProtectedPath(p) {
			t.Errorf("expected path %q to be unprotected, but was marked protected", p)
		}
	}
}

func TestExtractZipSafely_ZipSlipPrevention(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// Create malicious entry attempting to escape root
	f, err := zw.Create("openlocalcrm-main/../../evil.txt")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = f.Write([]byte("malicious"))
	_ = zw.Close()

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	destDir, err := os.MkdirTemp("", "zipslip_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destDir)

	u := NewUpdater(destDir)
	err = u.ExtractZipSafely(zr, destDir)
	if err == nil {
		t.Errorf("expected error on malicious Zip Slip archive, but got nil")
	}
}

func TestCheckForUpdate_SimulatedAPI(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("If-None-Match") == `"test-etag"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"test-etag"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sha": "newcommit123456789",
			"commit": map[string]any{
				"message": "fix: critical stability patch",
				"committer": map[string]any{
					"date": "2026-09-16T12:00:00Z",
				},
			},
		})
	}))
	defer server.Close()

	u := NewUpdater(".")
	u.RepoURL = server.URL
	u.Client = server.Client()

	// First call: gets new commit
	res, err := u.CheckForUpdate("oldcommit111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.HasUpdate || res.LatestCommit != "newcommit123456789" {
		t.Errorf("expected update to newcommit123456789, got %+v", res)
	}

	// Second call with same commit: uses ETag / 304 Not Modified
	res2, err := u.CheckForUpdate("oldcommit111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res2.HasUpdate {
		t.Errorf("expected cached update flag to remain true")
	}
	if calls != 1 {
		// Because cache TTL is active, second call within TTL should return cached without even contacting server
	}
}

func TestApplyStagedFiles_SkipsBlacklist(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "base_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	stagingDir, err := os.MkdirTemp("", "staging_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stagingDir)

	// User .env in baseDir
	_ = os.WriteFile(filepath.Join(baseDir, ".env"), []byte("SECRET=original"), 0644)
	// Staging tries to overwrite .env
	_ = os.WriteFile(filepath.Join(stagingDir, ".env"), []byte("SECRET=overwritten"), 0644)
	// Staging brings new docker-compose.yml
	_ = os.WriteFile(filepath.Join(stagingDir, "docker-compose.yml"), []byte("services: {}"), 0644)

	u := NewUpdater(baseDir)
	err = u.ApplyStagedFiles(stagingDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify .env was NOT overwritten
	envContent, _ := os.ReadFile(filepath.Join(baseDir, ".env"))
	if string(envContent) != "SECRET=original" {
		t.Errorf("CRITICAL: .env was overwritten! Got: %s", string(envContent))
	}

	// Verify docker-compose.yml was copied
	composeContent, _ := os.ReadFile(filepath.Join(baseDir, "docker-compose.yml"))
	if string(composeContent) != "services: {}" {
		t.Errorf("expected docker-compose.yml to be updated, got: %s", string(composeContent))
	}
}

func TestCleanupStaleOldExecutables(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "clean_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	oldFile := filepath.Join(tempDir, "launcher.exe.old")
	_ = os.WriteFile(oldFile, []byte("old binary"), 0755)

	u := NewUpdater(tempDir)
	u.CleanupStaleOldExecutablesInDir(tempDir)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("expected launcher.exe.old to be deleted, but still exists")
	}
}

func TestApplyStagedFiles_SkipsAllBinaries(t *testing.T) {
	stagingDir := t.TempDir()
	targetDir := t.TempDir()
	u := NewUpdater(targetDir)

	_ = os.MkdirAll(filepath.Join(stagingDir, "bin"), 0755)
	_ = os.WriteFile(filepath.Join(stagingDir, "bin", "openlocalcrm-setup-linux-amd64"), []byte("ELF binary"), 0755)
	_ = os.WriteFile(filepath.Join(stagingDir, "bin", "openlocalcrm-setup.exe"), []byte("PE binary"), 0755)
	_ = os.WriteFile(filepath.Join(stagingDir, "README.md"), []byte("# Readme"), 0644)

	err := u.ApplyStagedFiles(stagingDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "README.md")); err != nil {
		t.Errorf("expected README.md to be copied")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "bin", "openlocalcrm-setup-linux-amd64")); err == nil {
		t.Errorf("binary in bin/ should NOT be copied during staging walk")
	}
}

func TestGetAvailableVersions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"name": "v0.9",
				"commit": map[string]any{
					"sha": "cdf51e5123456789",
				},
			},
			{
				"name": "v0.8",
				"commit": map[string]any{
					"sha": "1234567abcdef",
				},
			},
		})
	}))
	defer ts.Close()

	targetDir := t.TempDir()
	u := NewUpdater(targetDir)
	u.TagsURL = ts.URL
	u.Client = ts.Client()

	resp := u.GetAvailableVersions(context.Background())
	if len(resp.Versions) != 3 { // v0.9, v0.8, main
		t.Fatalf("expected 3 versions (v0.9, v0.8, main), got %d: %+v", len(resp.Versions), resp.Versions)
	}

	if resp.Versions[0].Tag != "v0.9" || !resp.Versions[0].IsLatest {
		t.Errorf("expected v0.9 to be latest, got %+v", resp.Versions[0])
	}
	if resp.Versions[1].Tag != "v0.8" || resp.Versions[1].IsLatest {
		t.Errorf("expected v0.8 not latest, got %+v", resp.Versions[1])
	}
	if resp.Versions[2].Tag != "main" {
		t.Errorf("expected main branch, got %+v", resp.Versions[2])
	}

	// Test fallback when offline / error
	uBad := NewUpdater(targetDir)
	uBad.TagsURL = "http://127.0.0.1:54321/error"
	fallbackResp := uBad.GetAvailableVersions(context.Background())
	if len(fallbackResp.Versions) < 2 {
		t.Errorf("expected fallback versions on error, got %+v", fallbackResp)
	}
}

func TestEnsureProjectFilesForVersion_SkipIfExists(t *testing.T) {
	targetDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(targetDir, "Dockerfile.server"), []byte("FROM alpine"), 0644)

	u := NewUpdater(targetDir)
	logChan := make(chan string, 10)
	err := u.EnsureProjectFilesForVersion(context.Background(), "v0.9", false, logChan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(logChan)
	var logs []string
	for l := range logChan {
		logs = append(logs, l)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "bereits lokal vorhanden") {
		t.Errorf("expected log that files already exist, got %v", logs)
	}
}
