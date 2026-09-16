# GitHub Self-Updater for OpenLocalCRM Windows Launcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the OpenLocalCRM Windows setup launcher binary to automatically check for updates on GitHub, self-update its own executable live using the Windows rename hot-swap pattern, safely update repository files from GitHub ZIP without overwriting user data, and provide UI controls and status badges.

**Architecture:** A standalone `Updater` service in `internal/launcher/updater.go` manages GitHub commit checking with ETag caching, downloads the repository zipball into a quarantined staging folder, checks Zip Slip safety, filters against a strict blacklist (`.env`, `backups/`, `.git/`), performs an ordered socket release and hot-swap on Windows, rebuilds Docker containers if running, and restarts the launcher seamlessly.

**Tech Stack:** Go 1.22 (`archive/zip`, `net/http`, `os/exec`), Windows Win32 file mechanics, embedded HTML/JS UI (`internal/launcher/ui/`), Docker Compose CLI.

## Global Constraints
- **Zero External Dependencies:** No Git CLI required on host machine.
- **Strict Blacklist:** `.env`, `.env.local`, `backups/`, `.git/`, volume data directories must NEVER be overwritten.
- **Safety First:** Automatic pre-update database backup (`openlocalcrm_backup_pre_update_*.sql`) and `.env` copy before applying updates.
- **Windows Port Safety:** Server listener must shut down and free TCP `:9099` before spawning the replacement executable.
- **Zip Slip Prevention:** All extracted zip paths must be verified with `filepath.Clean` and `strings.HasPrefix`.
- **Legacy Compatibility:** Seamlessly updates whether named `openlocalcrm-setup.exe`, `openlocalcrm-setup-debug.exe`, or legacy `mavalio-setup.exe`.

---

### Task 1: Protected Path Filtering & Safe Zip Extraction with Zip-Slip Protection

**Files:**
- Create: `internal/launcher/updater.go`
- Create: `internal/launcher/updater_test.go`

**Interfaces:**
- Produces:
  ```go
  type Updater struct {
      BaseDir string
      Client  *http.Client
  }
  func NewUpdater(baseDir string) *Updater
  func (u *Updater) IsProtectedPath(relPath string) bool
  func (u *Updater) ExtractZipSafely(zipReader *zip.Reader, destDir string) error
  ```

- [ ] **Step 1: Write the failing unit tests for protected blacklist and Zip Slip**

Write `internal/launcher/updater_test.go`:
```go
package launcher

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestIsProtectedPath`
Expected: FAIL (NewUpdater not defined)

- [ ] **Step 3: Implement minimal code in `internal/launcher/updater.go`**

Write `internal/launcher/updater.go`:
```go
package launcher

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var protectedPaths = []string{
	".env",
	".env.local",
	".env.production",
	"backups",
	"data",
	"storage",
	".git",
	"openlocalcrm-setup.log",
	"caddy/certs",
}

type Updater struct {
	BaseDir string
	Client  *http.Client
}

func NewUpdater(baseDir string) *Updater {
	return &Updater{
		BaseDir: baseDir,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (u *Updater) IsProtectedPath(relPath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if clean == "." || clean == "/" {
		return false
	}
	if strings.HasPrefix(clean, "./") {
		clean = strings.TrimPrefix(clean, "./")
	}

	if strings.HasSuffix(clean, ".old") {
		return true
	}

	for _, p := range protectedPaths {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}

func (u *Updater) ExtractZipSafely(zipReader *zip.Reader, destDir string) error {
	cleanDest := filepath.Clean(destDir)

	for _, file := range zipReader.File {
		cleanName := filepath.Clean(file.Name)
		parts := strings.Split(filepath.ToSlash(cleanName), "/")
		if len(parts) <= 1 {
			continue // skip root container dir itself
		}

		relPath := strings.Join(parts[1:], "/")
		targetPath := filepath.Clean(filepath.Join(cleanDest, filepath.FromSlash(relPath)))

		// Zip Slip Guard
		if !strings.HasPrefix(targetPath, cleanDest+string(os.PathSeparator)) && targetPath != cleanDest {
			return fmt.Errorf("illegal path traversal detected in zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/launcher -run "TestIsProtectedPath|TestExtractZipSafely_ZipSlipPrevention"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/updater.go internal/launcher/updater_test.go
git commit -m "feat(updater): add protected blacklist and safe zip extraction with zip-slip guard"
```

---

### Task 2: GitHub Commit & Update Checker with ETag & Rate-Limit Caching

**Files:**
- Modify: `internal/launcher/updater.go`
- Modify: `internal/launcher/updater_test.go`

**Interfaces:**
- Produces:
  ```go
  type UpdateCheckResult struct {
      HasUpdate     bool      `json:"has_update"`
      CurrentCommit string    `json:"current_commit"`
      LatestCommit  string    `json:"latest_commit"`
      CommitMessage string    `json:"commit_message"`
      CommitDate    string    `json:"commit_date"`
      CheckedAt     time.Time `json:"checked_at"`
      RateLimited   bool      `json:"rate_limited"`
      Error         string    `json:"error,omitempty"`
  }
  func (u *Updater) CheckForUpdate(currentCommit string) (*UpdateCheckResult, error)
  ```

- [ ] **Step 1: Write failing unit test for GitHub commit checker with simulated ETag**

Add to `internal/launcher/updater_test.go`:
```go
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
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestCheckForUpdate_SimulatedAPI`
Expected: FAIL

- [ ] **Step 3: Implement Commit Checker in `internal/launcher/updater.go`**

Add caching and GitHub Commit check to `internal/launcher/updater.go`:
```go
type UpdateCheckResult struct {
	HasUpdate     bool      `json:"has_update"`
	CurrentCommit string    `json:"current_commit"`
	LatestCommit  string    `json:"latest_commit"`
	CommitMessage string    `json:"commit_message"`
	CommitDate    string    `json:"commit_date"`
	CheckedAt     time.Time `json:"checked_at"`
	RateLimited   bool      `json:"rate_limited"`
	Error         string    `json:"error,omitempty"`
}

type cachedCheck struct {
	etag      string
	result    *UpdateCheckResult
	timestamp time.Time
}

// In Updater struct:
// RepoURL string (defaults to "https://api.github.com/repos/boreddev1/openlocalcrm/commits/main")
// cacheMu sync.RWMutex
// cache *cachedCheck
```

Implement `CheckForUpdate(currentCommit string) (*UpdateCheckResult, error)` with ETag handling, 15-minute TTL cache, and rate-limit safety.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestCheckForUpdate_SimulatedAPI`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/updater.go internal/launcher/updater_test.go
git commit -m "feat(updater): add github commit checker with ETag and rate-limit caching"
```

---

### Task 3: Full Update Pipeline Execution & Windows Executable Hot-Swap

**Files:**
- Modify: `internal/launcher/updater.go`
- Modify: `internal/launcher/updater_test.go`

**Interfaces:**
- Produces:
  ```go
  func (u *Updater) ApplyStagedFiles(stagingDir string, logChan chan<- string) error
  func (u *Updater) HotSwapExecutable(stagingDir string, logChan chan<- string) (string, error)
  func (u *Updater) CleanupStaleOldExecutables()
  func (u *Updater) ExecuteUpdate(ctx context.Context, engine *Engine, logChan chan<- string) error
  ```

- [ ] **Step 1: Write unit tests for staging copy and cleanup of `.old` executables**

Add tests in `internal/launcher/updater_test.go`:
```go
func TestApplyStagedFiles_SkipsBlacklist(t *testing.T) {
	baseDir, _ := os.MkdirTemp("", "base_test")
	defer os.RemoveAll(baseDir)
	stagingDir, _ := os.MkdirTemp("", "staging_test")
	defer os.RemoveAll(stagingDir)

	// User .env in baseDir
	_ = os.WriteFile(filepath.Join(baseDir, ".env"), []byte("SECRET=original"), 0644)
	// Staging tries to overwrite .env
	_ = os.WriteFile(filepath.Join(stagingDir, ".env"), []byte("SECRET=overwritten"), 0644)
	// Staging brings new docker-compose.yml
	_ = os.WriteFile(filepath.Join(stagingDir, "docker-compose.yml"), []byte("services: {}"), 0644)

	u := NewUpdater(baseDir)
	err := u.ApplyStagedFiles(stagingDir, nil)
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
	tempDir, _ := os.MkdirTemp("", "clean_test")
	defer os.RemoveAll(tempDir)

	oldFile := filepath.Join(tempDir, "launcher.exe.old")
	_ = os.WriteFile(oldFile, []byte("old binary"), 0755)

	u := NewUpdater(tempDir)
	u.CleanupStaleOldExecutablesInDir(tempDir)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("expected launcher.exe.old to be deleted, but still exists")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run "TestApplyStagedFiles_SkipsBlacklist|TestCleanupStaleOldExecutables"`
Expected: FAIL

- [ ] **Step 3: Implement ApplyStagedFiles, HotSwapExecutable, and CleanupStaleOldExecutables**

Implement:
- `ApplyStagedFiles(stagingDir string, logChan chan<- string) error`: Walks staging directory, checks `IsProtectedPath`, backs up `.env` to `.env.backup.<ts>`, copies new files.
- `HotSwapExecutable(stagingDir string, logChan chan<- string) (string, error)`:
  Detects current executable path and base name (`openlocalcrm-setup.exe`, `openlocalcrm-setup-debug.exe`, or `mavalio-setup.exe`).
  Renames `currentExe` to `currentExe.old`.
  Copies matching binary from staging `bin/`.
- `CleanupStaleOldExecutables()`: Removes `*.old` files upon startup.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/launcher -run "TestApplyStagedFiles_SkipsBlacklist|TestCleanupStaleOldExecutables"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/updater.go internal/launcher/updater_test.go
git commit -m "feat(updater): implement staging file application and executable hot-swap logic"
```

---

### Task 4: HTTP API Endpoints in Launcher Server

**Files:**
- Modify: `internal/launcher/server.go`
- Modify: `cmd/setup-launcher/main.go`

**Interfaces:**
- Exposes:
  - `GET /api/update/check` -> returns `UpdateCheckResult`
  - `POST /api/update/execute` -> starts update, streams logs, orders graceful shutdown and spawns new binary

- [ ] **Step 1: Write test for `/api/update/check` route in server**

Add route test in `internal/launcher/server_test.go` (or create if not present):
```go
func TestServer_UpdateCheckRoute(t *testing.T) {
	engine := NewEngine(".")
	server := NewServer(".", engine)

	req := httptest.NewRequest(http.MethodGet, "/api/update/check", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected HTTP 200, got %d", w.Code)
	}
	var res UpdateCheckResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestServer_UpdateCheckRoute`
Expected: FAIL (404 Not Found)

- [ ] **Step 3: Wire Updater and Handlers in `internal/launcher/server.go`**

Register routes:
```go
mux.HandleFunc("/api/update/check", s.handleUpdateCheck)
mux.HandleFunc("/api/update/execute", s.handleUpdateExecute)
```
Implement `handleUpdateCheck` and `handleUpdateExecute`:
- `handleUpdateCheck`: Invokes `s.updater.CheckForUpdate(BuildCommit)`.
- `handleUpdateExecute`: Starts update pipeline in background. When complete, notifies clients, shuts down HTTP listener to release port 9099, spawns new executable via `exec.Command`, and exits with `os.Exit(0)`.
In `cmd/setup-launcher/main.go`: call `updater.CleanupStaleOldExecutables()` on startup.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestServer_UpdateCheckRoute`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/server.go cmd/setup-launcher/main.go internal/launcher/server_test.go
git commit -m "feat(launcher): add update/check and update/execute API endpoints with socket release"
```

---

### Task 5: Launcher Embedded Web UI Integration

**Files:**
- Modify: `internal/launcher/ui/index.html`
- Modify: `internal/launcher/ui/app.js`
- Modify: `internal/launcher/ui/style.css`

**UI Features:**
1. Header badge: Shows version and update button (`⚡ Update verfügbar`) when a newer version exists.
2. Control Center: "🔄 Nach Updates suchen" button in the action bar.
3. Update modal: Shows changelog/commit message, explains that automatic DB backup will be made, and includes "Jetzt aktualisieren" button.
4. Auto-reconnect loop: When updating, UI displays progress, waits for the new server on `:9099` to become available, and refreshes the browser.

- [ ] **Step 1: Add Update elements and modal to `internal/launcher/ui/index.html`**

Add update badge in `<header>` and update action modal in `index.html`.

- [ ] **Step 2: Add styles in `internal/launcher/ui/style.css`**

Add styling for `.badge-update`, pulse animation, and modal dialog.

- [ ] **Step 3: Implement client-side update logic in `internal/launcher/ui/app.js`**

Implement:
- `checkForUpdates()` on startup and on button click.
- `openUpdateModal()` and `executeSystemUpdate()`.
- `pollForRestartAndReload()` with healthcheck backoff.

- [ ] **Step 4: Verify embedded UI build and tests**

Run: `go test -v ./internal/launcher/ui/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/ui/index.html internal/launcher/ui/app.js internal/launcher/ui/style.css
git commit -m "feat(launcher-ui): integrate update badge, modal and auto-reconnect in launcher UI"
```

---

### Task 6: Build Pipeline, `-ldflags` Commit Injection & Verification

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/docker-publish.yml`

- [ ] **Step 1: Update `Makefile` to inject `BuildCommit` and `BuildDate`**

In `Makefile`:
```make
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -w -s -X "github.com/openlocalcrm/openlocalcrm/internal/launcher.BuildCommit=$(GIT_COMMIT)" -X "github.com/openlocalcrm/openlocalcrm/internal/launcher.BuildDate=$(BUILD_DATE)"

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS) -H=windowsgui" -o bin/openlocalcrm-setup.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup.exe bin/mavalio-setup.exe

build-windows-debug:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/openlocalcrm-setup-debug.exe ./cmd/setup-launcher
	cp bin/openlocalcrm-setup-debug.exe bin/mavalio-setup-debug.exe
```

- [ ] **Step 2: Update `.github/workflows/docker-publish.yml` to compile Windows launcher binaries**

Add Windows binary build step to GitHub Actions workflow.

- [ ] **Step 3: Run complete verification**

Run:
1. `go test -v ./...`
2. `make build-windows`
3. `make build-windows-debug`
Expected: All tests pass, binaries compile with injected version metadata.

- [ ] **Step 4: Commit**

```bash
git add Makefile .github/workflows/docker-publish.yml
git commit -m "ci: inject build commit metadata in launcher binaries and add windows builds to release workflow"
```
