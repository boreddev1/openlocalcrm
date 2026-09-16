# Windows Robust Setup, WSL2/Docker Auto-Install, Container-Updates & Backup-Import Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the Windows Setup & Control Launcher (`openlocalcrm-setup.exe`, Alias: `mavalio-setup.exe`) into a battle-hardened, self-healing tool that:
1. Fixes the 5 fundamental installer bugs (path resolution from `bin/`, hardcoded port 80, database credential mismatch, missing embedded templates, fake 30s timeout).
2. Performs robust detection of WSL2 and Docker Desktop, and triggers automated 1-click installations with Windows UAC elevation (`Start-Process ... -Verb RunAs`).
3. Supports 1-click container rebuilds (`docker compose up -d --build`) on code updates with automatic pre-update safety snapshots.
4. Supports importing and restoring database backups (`.sql` dumps) both during initial setup and in the Control Center.

**Architecture:** 
- `Preflight & Windows Elevation`: Detects WSL2 status (`wsl --status`) and Docker Engine. Uses PowerShell `-Verb RunAs` to elevate installations of WSL (`wsl --install --no-distribution`) and Docker Desktop (`winget install -e --id Docker.DockerDesktop`).
- `Path & Template Engine`: Auto-detects project root (`.` or `..` if in `bin/`) and embeds fallback `docker-compose.yml` + `Caddyfile` so the binary works even standalone without a Git clone.
- `Dynamic Compose`: Configures `docker-compose.yml` to use `${APP_PORT:-80}:80`, `${DB_USER}`, `${DB_PASSWORD}`, `${DB_NAME}`, and `${DATABASE_URL}` matching `config.go`.
- `Engine & Server`: Orchestrates `Update()` (auto-backup + rebuild) and `RestoreBackup()` (`psql` pipe into container), exposed via `/api/control/update`, `/api/backups`, `/api/backup/upload`, `/api/backup/restore`, `/api/install/wsl`, and `/api/install/docker`.
- `UI`: Replaces fake frontend timeouts with genuine status reporting, adds 1-click install buttons for WSL/Docker, and provides full backup import and container rebuild dashboards.

**Tech Stack:** Go 1.22+, PowerShell (`-Verb RunAs`), Win32 Syscalls, Docker Compose CLI, PostgreSQL `pg_dump` & `psql`, HTML5/CSS3/Vanilla JS (`embed.FS`).

## Global Constraints
- Target binary: `bin/openlocalcrm-setup.exe` (GUI, Alias: `mavalio-setup.exe`) and `bin/openlocalcrm-setup-debug.exe` (Console).
- Zero CGO: Pure Go with Win32 Syscalls, cross-compiles cleanly with `CGO_ENABLED=0`.
- Data Safety: Automatic database backup must always be created before performing a container update or restore.
- Process Hiding: All background commands must maintain `CREATE_NO_WINDOW (0x08000000)` on Windows, while elevation commands explicitly trigger Windows UAC via PowerShell.
- Path Isolation: Restores must strictly sanitize filenames with `filepath.Base` to prevent directory traversal.

---

### Phase 1: Fundament-Reparatur & Windows-Voraussetzungen (WSL2 & Docker)

---

### Task 1: WSL2 & Docker Desktop Auto-Detection and 1-Click Elevation

**Files:**
- Modify: `internal/launcher/preflight.go`
- Test: `internal/launcher/preflight_test.go`

**Interfaces:**
- Produces:
  ```go
  type SystemStatus struct {
      WSLInstalled    bool   `json:"wsl_installed"`
      WSLStatus       string `json:"wsl_status"`
      DockerInstalled bool   `json:"docker_installed"`
      DockerRunning   bool   `json:"docker_running"`
      DockerVersion   string `json:"docker_version"`
      Port80Free      bool   `json:"port_80_free"`
      Port8080Free    bool   `json:"port_8080_free"`
      SuggestedPort   int    `json:"suggested_port"`
  }
  func RunPreflightCheck(ctx context.Context) SystemStatus
  func CheckWSLStatus(ctx context.Context) (bool, string)
  func TriggerWSLInstall(ctx context.Context) error
  func TriggerDockerInstall(ctx context.Context) error
  func TriggerDockerStart(ctx context.Context) error
  ```

- [ ] **Step 1: Write failing tests for WSL and Docker status parsing**

In `internal/launcher/preflight_test.go`:
```go
func TestParseWSLStatus(t *testing.T) {
	cases := []struct {
		output    string
		installed bool
	}{
		{"Default Version: 2\nWindows Subsystem for Linux is running.", true},
		{"WSL is not installed.", false},
		{"", false},
	}

	for _, c := range cases {
		installed := parseWSLOutput(c.output)
		if installed != c.installed {
			t.Errorf("parseWSLOutput(%q) = %v; want %v", c.output, installed, c.installed)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestParseWSLStatus`
Expected: FAIL with compilation error (undefined: parseWSLOutput)

- [ ] **Step 3: Implement WSL2 check, Docker detection, and PowerShell RunAs elevation**

In `internal/launcher/preflight.go`:
- Implement `parseWSLOutput(output string) bool`
- Implement `CheckWSLStatus(ctx context.Context) (bool, string)`: Runs `wsl --status`. If missing, returns false.
- Implement `TriggerWSLInstall(ctx context.Context) error`:
  Executes `powershell.exe -NoProfile -Command "Start-Process wsl -ArgumentList '--install --no-distribution' -Verb RunAs"`.
- Implement `TriggerDockerInstall(ctx context.Context) error`:
  Executes `powershell.exe -NoProfile -Command "Start-Process winget -ArgumentList 'install -e --id Docker.DockerDesktop --accept-package-agreements --accept-source-agreements' -Verb RunAs"`.
- Implement `TriggerDockerStart(ctx context.Context) error`:
  Looks for `C:\Program Files\Docker\Docker\Docker Desktop.exe` and starts it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestParseWSLStatus`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/preflight.go internal/launcher/preflight_test.go
git commit -m "feat(launcher): implement WSL2 check and PowerShell UAC elevation for WSL and Docker"
```

---

### Task 2: Robust BaseDir Resolution & Embedded Compose Fallback

**Files:**
- Modify: `cmd/setup-launcher/main.go`
- Create: `internal/launcher/templates.go`
- Test: `internal/launcher/templates_test.go`

**Interfaces:**
- Produces:
  ```go
  func EnsureComposeAndCaddyFiles(baseDir string) error
  func ResolveProjectBaseDir() string
  ```

- [ ] **Step 1: Write failing test for EnsureComposeAndCaddyFiles**

In `internal/launcher/templates_test.go`:
```go
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
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestEnsureComposeAndCaddyFiles`
Expected: FAIL with compilation error (undefined: EnsureComposeAndCaddyFiles)

- [ ] **Step 3: Implement ResolveProjectBaseDir and embedded compose fallback**

Create `internal/launcher/templates.go`:
- Embed default `docker-compose.yml` and `Caddyfile` templates.
- Implement `EnsureComposeAndCaddyFiles(baseDir string)`:
  If `docker-compose.yml` does not exist in `baseDir`, write out the embedded default.
  If `Caddyfile` does not exist in `baseDir`, write out the embedded default.

In `cmd/setup-launcher/main.go`:
- Fix baseDir resolution:
  ```go
  func ResolveProjectBaseDir() string {
      // 1. If docker-compose.yml exists in current working dir, use it
      if _, err := os.Stat("docker-compose.yml"); err == nil {
          return "."
      }
      // 2. If running from bin/ and parent has docker-compose.yml, use parent
      if exePath, err := os.Executable(); err == nil {
          exeDir := filepath.Dir(exePath)
          if _, err := os.Stat(filepath.Join(exeDir, "docker-compose.yml")); err == nil {
              return exeDir
          }
          parentDir := filepath.Dir(exeDir)
          if _, err := os.Stat(filepath.Join(parentDir, "docker-compose.yml")); err == nil {
              return parentDir
          }
          return exeDir
      }
      return "."
  }
  ```
- In `main()`: Call `EnsureComposeAndCaddyFiles(baseDir)` immediately after resolving `baseDir`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestEnsureComposeAndCaddyFiles`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/setup-launcher/main.go internal/launcher/templates.go internal/launcher/templates_test.go
git commit -m "fix(launcher): resolve baseDir correctly from bin/ and add embedded compose fallback"
```

---

### Task 3: Dynamic Ports & Database Synchronization

**Files:**
- Modify: `docker-compose.yml`
- Modify: `internal/launcher/config.go`
- Modify: `internal/launcher/config_test.go`

- [ ] **Step 1: Write failing test verifying config.go produces DB_HOST=db and APP_PORT**

In `internal/launcher/config_test.go`:
```go
func TestGenerateEnvContent_DatabaseSync(t *testing.T) {
	cfg := SetupConfig{
		Port:          8080,
		AdminEmail:    "admin@mavalio.local",
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
	if !strings.Contains(content, "@db:5432/mavalio") {
		t.Errorf("expected @db:5432/mavalio in DATABASE_URL")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestGenerateEnvContent_DatabaseSync`
Expected: FAIL

- [ ] **Step 3: Update docker-compose.yml and config.go**

In `docker-compose.yml`:
- In `caddy` service:
  Change ports to:
  ```yaml
  ports:
    - "${APP_PORT:-80}:80"
    - "443:443"
  ```
- In `server` service:
  ```yaml
  environment:
    - PORT=8080
    - DATABASE_URL=${DATABASE_URL:-postgres://postgres:crm_pass@db:5432/mavalio?sslmode=disable}
    - STORAGE_PATH=/data/storage
    - JWT_SECRET_KEY_PATH=/app/keys/ed25519.key
    - DEMO_MODE=${DEMO_MODE:-false}
  ```
- In `db` service:
  ```yaml
  environment:
    - POSTGRES_USER=${DB_USER:-postgres}
    - POSTGRES_PASSWORD=${DB_PASSWORD:-crm_pass}
    - POSTGRES_DB=${DB_NAME:-mavalio}
  ```

In `internal/launcher/config.go`:
- Change `DB_HOST=crm-db` to `DB_HOST=db`.
- Add `APP_PORT=%d`.
- Set `DATABASE_URL=postgres://postgres:%s@db:5432/mavalio?sslmode=disable`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestGenerateEnvContent_DatabaseSync`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml internal/launcher/config.go internal/launcher/config_test.go
git commit -m "fix(compose): parameterize APP_PORT and synchronize database credentials with .env"
```

---

### Phase 2: Updates, Backups & UI

---

### Task 4: Engine Backup List, Restore & Container Update

**Files:**
- Modify: `internal/launcher/engine.go`
- Test: `internal/launcher/engine_test.go`

**Interfaces:**
- Produces:
  ```go
  type BackupInfo struct {
      Filename      string    `json:"filename"`
      SizeBytes     int64     `json:"size_bytes"`
      SizeFormatted string    `json:"size_formatted"`
      CreatedAt     time.Time `json:"created_at"`
  }
  func (e *Engine) ListBackups() ([]BackupInfo, error)
  func (e *Engine) RestoreBackup(ctx context.Context, filename string, logChan chan<- string) error
  func (e *Engine) Update(ctx context.Context, logChan chan<- string) error
  ```

- [ ] **Step 1: Write failing tests for ListBackups and RestoreBackup filename validation**

In `internal/launcher/engine_test.go`:
```go
func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	backupsDir := filepath.Join(tmpDir, "backups")
	_ = os.MkdirAll(backupsDir, 0755)
	_ = os.WriteFile(filepath.Join(backupsDir, "test_backup.sql"), []byte("SELECT 1;"), 0644)

	list, err := engine.ListBackups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(list))
	}
	if list[0].Filename != "test_backup.sql" {
		t.Errorf("expected filename test_backup.sql, got %s", list[0].Filename)
	}
}

func TestRestoreBackup_SecurityTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	err := engine.RestoreBackup(context.Background(), "../secret.sql", nil)
	if err == nil {
		t.Errorf("expected error on directory traversal filename")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run "TestListBackups|TestRestoreBackup_SecurityTraversal"`
Expected: FAIL

- [ ] **Step 3: Implement ListBackups, RestoreBackup, and Update in Engine**

In `internal/launcher/engine.go`:
- Implement `ListBackups()`
- Implement `RestoreBackup()` (pipes SQL into `docker compose exec -T db psql -U postgres -d mavalio`)
- Implement `Update()` (creates safety snapshot + executes `docker compose up -d --build --remove-orphans`)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run "TestListBackups|TestRestoreBackup_SecurityTraversal"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/engine.go internal/launcher/engine_test.go
git commit -m "feat(launcher): implement backup listing, SQL restore, and container update in engine"
```

---

### Task 5: REST API Endpoints for WSL/Docker Install, Backups & Updates

**Files:**
- Modify: `internal/launcher/server.go`
- Test: `internal/launcher/server_test.go`

**Endpoints:**
- `POST /api/install/wsl`: Triggers `TriggerWSLInstall`
- `POST /api/install/docker`: Triggers `TriggerDockerInstall`
- `POST /api/install/start-docker`: Triggers `TriggerDockerStart`
- `GET /api/backups`: Returns list of `.sql` files
- `POST /api/backup/upload`: Multipart upload of `.sql` file
- `POST /api/backup/restore`: Accepts `{"filename": "..."}`, executes `engine.RestoreBackup`
- `POST /api/control/update`: Executes `engine.Update` with live logs

- [ ] **Step 1: Write failing tests for backup list and install endpoints**

In `internal/launcher/server_test.go`:
```go
func TestServerBackupsListEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("GET", "/api/backups", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestServerBackupsListEndpoint`
Expected: FAIL

- [ ] **Step 3: Implement new routes and handlers in server.go**

In `internal/launcher/server.go`:
- Wire `/api/install/wsl`, `/api/install/docker`, `/api/install/start-docker`.
- Wire `/api/backups`, `/api/backup/upload`, `/api/backup/restore`.
- Wire `/api/control/update`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestServerBackupsListEndpoint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/server.go internal/launcher/server_test.go
git commit -m "feat(launcher): add WSL/Docker install, backup management, and update REST endpoints"
```

---

### Task 6: UI Enhancements (WSL/Docker 1-Click Installers, Backup Import & Update Center)

**Files:**
- Modify: `internal/launcher/ui/index.html`
- Modify: `internal/launcher/ui/app.js`
- Modify: `internal/launcher/ui/style.css`

- [ ] **Step 1: Update index.html**
  - In Preflight (Step 1): Add WSL2 status card. If WSL is missing, show button "🚀 WSL2 jetzt installieren (Admin UAC)". If Docker is missing, show "🐳 Docker Desktop installieren (Admin UAC)".
  - In Wizard (Step 2): Add accordion "📦 Bestehende Datenbank importieren (.sql Dump)" with file upload.
  - In Step 3 (Deployment): Eliminate fake 30-second timeout! Show real status and error banner with retry if polling times out after 120s.
  - In Control Center:
    - Primary button "🔄 System & Container aktualisieren (Rebuild)".
    - Backup Management table with "Erstellen", "Wiederherstellen" and "Datei hochladen".

- [ ] **Step 2: Update app.js**
  - Functions `installWSL()`, `installDocker()`, `startDocker()` calling respective APIs.
  - Real health check without false completion.
  - Functions `fetchBackups()`, `uploadBackup()`, `restoreBackup()`, and `updateContainers()`.

- [ ] **Step 3: Verify embedded UI tests pass**

Run: `go test -v ./internal/launcher/ui`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/launcher/ui/
git commit -m "feat(launcher): enhance UI with 1-click WSL/Docker installers, backup manager, and update center"
```

---

### Task 7: Windows Binaries Recompile & Comprehensive Verification

**Files:**
- Recompile: `bin/mavalio-setup.exe`
- Recompile: `bin/mavalio-setup-debug.exe`

- [ ] **Step 1: Run full unit test suite**

Run: `go test -v ./internal/launcher/...`
Expected: All tests pass.

- [ ] **Step 2: Build both Windows binaries via Makefile**

Run: `make build-windows && make build-windows-debug`
Expected: Both binaries recompiled.

- [ ] **Step 3: Verify binary headers and sizes**

Run: `file bin/mavalio-setup*.exe`
Expected: Valid PE32+ executables.

- [ ] **Step 4: Commit binaries and updated implementation**

```bash
git add bin/mavalio-setup.exe bin/mavalio-setup-debug.exe
git commit -m "feat(build): compile updated Windows release and debug binaries with WSL2 auto-install and backup import"
```

---

## Execution Choice

Two execution options:
1. **Subagent-Driven (recommended)** - Execute each task via subagent with two-stage reviews.
2. **Inline Execution** - Execute tasks directly in this session with immediate TDD verifications.
