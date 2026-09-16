# Universal Cross-Platform GUI & CLI Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single cross-platform executable (`openlocalcrm` / `openlocalcrm-setup`) supporting macOS, Linux, and Windows that launches the browser-based setup wizard when invoked without arguments (or in GUI mode), and directly executes terminal CLI subcommands (`status`, `install`, `start`, `stop`, `restart`, `update`, `backup`, `reset`, `admin`) when arguments are provided, complete with OS-adaptive preflight, headless SSH detection, atomic Unix binary updates, and a one-liner curl installer script.

**Architecture:** A unified binary using Go standard library with zero external dependencies. Subcommand routing in `cmd/setup-launcher/cli.go` drives the existing `internal/launcher/engine.go` operations. Preflight checks in `internal/launcher/preflight.go` report OS and CPU architecture to dynamically adapt the Web GUI DOM (hiding WSL on macOS/Linux and tailoring Docker hints). The self-updater in `internal/launcher/updater.go` performs atomic inode replacement (`os.Rename`) on Unix, and `scripts/install.sh` provides a universal installer.

**Tech Stack:** Go 1.22 (`os/exec`, `net/http`, `archive/zip`, `syscall`, `runtime`), Docker Compose v2, Vanilla HTML5/CSS/JavaScript (embedded via `embed.FS`), POSIX Shell (`scripts/install.sh`).

## Global Constraints
- **Single All-in-One Binary:** One unified codebase for both the Web GUI server and the CLI command-line tool.
- **Zero External Go Dependencies:** Pure standard library for CLI argument parsing, HTTP serving, and process execution.
- **100% Backward Compatibility:** Windows setup launcher, Win32 tray, and existing instances must continue working without any regression.
- **Headless Safety:** Never invoke `xdg-open` or attempt GUI actions when running on a headless Linux host (`$DISPLAY` and `$WAYLAND_DISPLAY` empty).
- **Strict Data Blacklist:** `.env`, `.env.local`, `backups/`, `.git/`, and persistent data directories must never be overwritten during self-updates.
- **Non-Root Privileged Port Handling:** Accurate detection of port 80 availability even when unprivileged non-root users test on macOS/Linux.

---

### Task 1: Preflight OS & Arch Exposure, WSL Bypass & Non-Root Port 80 Check

**Files:**
- Modify: `internal/launcher/preflight.go`
- Modify: `internal/launcher/preflight_test.go`

**Interfaces:**
- Produces:
  ```go
  type SystemStatus struct {
      OS              string `json:"os"`
      Arch            string `json:"arch"`
      WSLInstalled    bool   `json:"wsl_installed"`
      WSLStatus       string `json:"wsl_status"`
      DockerInstalled bool   `json:"docker_installed"`
      DockerRunning   bool   `json:"docker_running"`
      DockerVersion   string `json:"docker_version"`
      Port80Free      bool   `json:"port_80_free"`
      Port8080Free    bool   `json:"port_8080_free"`
      SuggestedPort   int    `json:"suggested_port"`
  }
  func CheckWSLStatus(ctx context.Context) (bool, string)
  func CheckPortAvailable(port int) bool
  func TriggerDockerStart(ctx context.Context) error
  ```

- [ ] **Step 1: Write failing unit tests for OS/Arch fields and non-Windows WSL bypass**

Update `internal/launcher/preflight_test.go`:
```go
func TestPreflight_OSAndArch(t *testing.T) {
	status := RunPreflightCheck(context.Background())
	if status.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, status.OS)
	}
	if status.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %s, got %s", runtime.GOARCH, status.Arch)
	}
}

func TestCheckWSLStatus_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	installed, msg := CheckWSLStatus(context.Background())
	if !installed {
		t.Errorf("expected WSL to be marked as installed/not required on non-windows, got false")
	}
	if !strings.Contains(strings.ToLower(msg), "not required") && !strings.Contains(strings.ToLower(msg), "nicht erforderlich") {
		t.Errorf("unexpected wsl message: %s", msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run "TestPreflight_OSAndArch|TestCheckWSLStatus_NonWindows"`
Expected: FAIL due to missing `status.OS` and `status.Arch` fields.

- [ ] **Step 3: Implement OS/Arch, WSL bypass, privileged port fallback, and macOS Docker start**

Update `internal/launcher/preflight.go`:
- Add `OS string json:"os"` and `Arch string json:"arch"` to `SystemStatus`.
- In `RunPreflightCheck`, set `status.OS = runtime.GOOS` and `status.Arch = runtime.GOARCH`.
- In `CheckPortAvailable(port int) bool`:
  ```go
  func CheckPortAvailable(port int) bool {
  	addr := fmt.Sprintf("127.0.0.1:%d", port)
  	ln, err := net.Listen("tcp", addr)
  	if err == nil {
  		_ = ln.Close()
  		return true
  	}
  	// If non-root user attempts port < 1024 on Unix, net.Listen returns permission denied.
  	// Test if anything is actively listening by connecting:
  	if runtime.GOOS != "windows" && port < 1024 && strings.Contains(strings.ToLower(err.Error()), "permission denied") {
  		conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
  		if dialErr != nil {
  			// Connection refused or timed out -> port is NOT occupied by any listening server!
  			return true
  		}
  		_ = conn.Close()
  		return false // Port is actively occupied
  	}
  	return false
  }
  ```
- In `TriggerDockerStart(ctx context.Context) error`:
  - If `runtime.GOOS == "darwin"`: run `exec.Command("open", "-a", "Docker").Run()`.
  - If `runtime.GOOS == "linux"`: run `exec.Command("systemctl", "start", "docker").Run()`.
  - If `runtime.GOOS == "windows"`: keep existing PowerShell start.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/launcher -run "TestPreflight_OSAndArch|TestCheckWSLStatus_NonWindows"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/preflight.go internal/launcher/preflight_test.go
git commit -m "feat(preflight): expose OS and arch, bypass WSL on Unix, support non-root port 80 probe"
```

---

### Task 2: Project Base Directory Resolution (`ResolveProjectBaseDir`)

**Files:**
- Modify: `cmd/setup-launcher/main.go`
- Create: `cmd/setup-launcher/dir.go`
- Create: `cmd/setup-launcher/dir_test.go`

**Interfaces:**
- Produces:
  ```go
  func ResolveProjectBaseDir(customDir string) string
  ```

- [ ] **Step 1: Write unit tests for directory resolution priority**

Create `cmd/setup-launcher/dir_test.go`:
```go
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
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	_ = os.Chdir(tempDir)

	got := ResolveProjectBaseDir("")
	if got != "." && got != tempDir {
		t.Errorf("expected . or %s, got %s", tempDir, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/setup-launcher -run "TestResolveProjectBaseDir"`
Expected: FAIL

- [ ] **Step 3: Implement `ResolveProjectBaseDir` in `dir.go`**

Create `cmd/setup-launcher/dir.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveProjectBaseDir determines the directory where configuration and docker files are stored.
func ResolveProjectBaseDir(customDir string) string {
	if customDir != "" {
		return customDir
	}

	if envDir := os.Getenv("OPENLOCALCRM_DIR"); envDir != "" {
		return envDir
	}

	// 1. Current working directory has docker-compose.yml or .env
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "."
	}
	if _, err := os.Stat(".env"); err == nil {
		return "."
	}

	// 2. Executable directory or parent directory
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if _, err := os.Stat(filepath.Join(exeDir, "docker-compose.yml")); err == nil {
			return exeDir
		}
		parentDir := filepath.Dir(exeDir)
		if _, err := os.Stat(filepath.Join(parentDir, "docker-compose.yml")); err == nil {
			return parentDir
		}

		// Windows standard candidates
		if runtime.GOOS == "windows" {
			candidates := []string{
				"C:\\openlocalcrm",
				"C:\\mavalio",
				filepath.Join(os.Getenv("LOCALAPPDATA"), "openlocalcrm"),
				filepath.Join(os.Getenv("LOCALAPPDATA"), "mavalio"),
				filepath.Join(os.Getenv("ProgramFiles"), "OpenLocalCRM"),
				filepath.Join(os.Getenv("ProgramFiles"), "mavalio CRM"),
			}
			for _, c := range candidates {
				if c != "" {
					if _, err := os.Stat(filepath.Join(c, "docker-compose.yml")); err == nil {
						return c
					}
					if _, err := os.Stat(filepath.Join(c, ".env")); err == nil {
						return c
					}
				}
			}
			return exeDir
		}

		// Unix standard candidates: ~/.openlocalcrm
		if home, hErr := os.UserHomeDir(); hErr == nil {
			userDir := filepath.Join(home, ".openlocalcrm")
			if _, err := os.Stat(filepath.Join(userDir, "docker-compose.yml")); err == nil {
				return userDir
			}
			if _, err := os.Stat(filepath.Join(userDir, ".env")); err == nil {
				return userDir
			}
		}

		// If running from system PATH (/usr/local/bin, /usr/bin, etc.), default project dir to ~/.openlocalcrm
		if exeDir == "/usr/local/bin" || exeDir == "/usr/bin" || exeDir == "/bin" {
			if home, hErr := os.UserHomeDir(); hErr == nil {
				userDir := filepath.Join(home, ".openlocalcrm")
				_ = os.MkdirAll(userDir, 0755)
				return userDir
			}
		}

		return exeDir
	}

	return "."
}
```
In `cmd/setup-launcher/main.go`, replace the old inline `ResolveProjectBaseDir()` with calls to the new function.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./cmd/setup-launcher -run "TestResolveProjectBaseDir"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/setup-launcher/dir.go cmd/setup-launcher/dir_test.go cmd/setup-launcher/main.go
git commit -m "feat(launcher): implement robust cross-platform project directory resolution"
```

---

### Task 3: CLI Subcommand Dispatcher & Headless Server Support

**Files:**
- Create: `cmd/setup-launcher/cli.go`
- Create: `cmd/setup-launcher/cli_test.go`
- Modify: `cmd/setup-launcher/main.go`

**Interfaces:**
- Produces:
  ```go
  func RunCLI(args []string, baseDir string) (bool, int)
  ```
  Returns `(handled bool, exitCode int)`. If `handled` is false, caller proceeds to launch Web GUI.

- [ ] **Step 1: Write unit tests for CLI argument routing**

Create `cmd/setup-launcher/cli_test.go`:
```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCLI_Help(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--help"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected help output to mention OpenLocalCRM, got: %s", buf.String())
	}
}

func TestRunCLI_Version(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--version"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "v3.0") && !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected version output, got: %s", buf.String())
	}
}

func TestRunCLI_UnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"foobar"}, t.TempDir(), &buf)
	if !handled || code != 2 {
		t.Fatalf("expected handled=true code=2 for unknown command, got %v, %d", handled, code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/setup-launcher -run "TestRunCLI"`
Expected: FAIL

- [ ] **Step 3: Implement CLI Subcommand Dispatcher in `cli.go`**

Implement `cmd/setup-launcher/cli.go`:
- Implement commands:
  - `help` / `--help` / `-h`
  - `version` / `--version` / `-v`
  - `status [--json]`
  - `install [flags]` (parses `--port`, `--admin-email`, `--admin-password`, `--ai-provider`, `--ai-url`, `--ai-model`, `--ai-key`, `--demo`, `-y`/`--yes`/`--non-interactive`, `--dir`)
  - `start`, `stop`, `restart`, `down`
  - `update`
  - `backup [create | list | restore <file>]`
  - `reset [-f | --force]` (checks `isatty` or prompts `[y/N]`)
  - `admin password <email> <password>`
  - `gui` (returns `handled: false` to start web server)
- Connect all engine methods cleanly.

- [ ] **Step 4: Update `main.go` for CLI dispatch, headless check, and cross-platform branding**

In `cmd/setup-launcher/main.go`:
- At start of `main()`:
  ```go
  if len(os.Args) > 1 && os.Args[1] != "gui" {
  	baseDir := ResolveProjectBaseDir(extractDirFlag(os.Args))
  	handled, code := RunCLI(os.Args[1:], baseDir)
  	if handled {
  		os.Exit(code)
  	}
  }
  ```
- In headless Linux check:
  ```go
  isHeadless := runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
  ```
  If `isHeadless`, skip `openBrowser` and log:
  ```
  log.Println("[Headless] Keine grafische Anzeige erkannt ($DISPLAY unset).")
  log.Printf("[Headless] Web-Oberfläche erreichbar unter: %s (z. B. via SSH-Tunnel).", serverURL)
  log.Println("[Headless] Nutzen Sie 'openlocalcrm --help' für die Steuerung im Terminal.")
  ```
- Change log banner from `OpenLocalCRM — Windows Setup & Control Launcher` to `OpenLocalCRM — Setup & Control Center`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./cmd/setup-launcher`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/setup-launcher/cli.go cmd/setup-launcher/cli_test.go cmd/setup-launcher/main.go
git commit -m "feat(cli): add CLI subcommand dispatcher and headless server support"
```

---

### Task 4: Cross-Platform Self-Updater & Atomic Inode Replacement

**Files:**
- Modify: `internal/launcher/updater.go`
- Modify: `internal/launcher/updater_test.go`

**Interfaces:**
- Produces:
  ```go
  func (u *Updater) HotSwapExecutable(stagingDir string, logChan chan<- string) (string, error)
  ```
  Safely swaps executable on macOS/Linux using atomic rename, and on Windows using the delayed `.old` swap.

- [ ] **Step 1: Write unit tests for staging walk ignoring `bin/` and target binary discovery**

Update `internal/launcher/updater_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run "TestApplyStagedFiles_SkipsAllBinaries"`
Expected: FAIL (binary is currently copied if not ending with `.exe`).

- [ ] **Step 3: Update `updater.go` for Unix atomic swap & candidate resolution**

In `internal/launcher/updater.go`:
- In `ApplyStagedFiles`: skip all files inside `bin/` or matching binary names:
  ```go
  slashRel := filepath.ToSlash(relPath)
  if slashRel == "bin" || strings.HasPrefix(slashRel, "bin/") {
  	if info.IsDir() {
  		return nil
  	}
  	// Skip all files in bin/ during recursive file walk
  	return nil
  }
  ```
- In `HotSwapExecutable`:
  - Search candidates based on `runtime.GOOS` and `runtime.GOARCH`:
    ```go
    targetPlatformName := fmt.Sprintf("openlocalcrm-setup-%s-%s", runtime.GOOS, runtime.GOARCH)
    if runtime.GOOS == "windows" {
    	targetPlatformName += ".exe"
    }
    candidates := []string{
    	filepath.Join(stagingDir, "bin", targetPlatformName),
    	filepath.Join(stagingDir, "bin", exeName),
    	filepath.Join(stagingDir, "bin", "openlocalcrm"),
    	filepath.Join(stagingDir, "bin", "openlocalcrm-setup.exe"),
    	filepath.Join(stagingDir, exeName),
    }
    ```
  - If `runtime.GOOS != "windows"`:
    - Atomically swap using temporary file and `os.Rename`:
      ```go
      tmpExe := currentExe + ".update_tmp"
      if err := copyFile(sourceBinary, tmpExe, 0755); err != nil {
      	if os.IsPermission(err) {
      		return "", fmt.Errorf("keine Schreibberechtigung für %s (bitte mit 'sudo openlocalcrm update' ausführen): %w", currentExe, err)
      	}
      	return "", fmt.Errorf("failed staging binary swap: %w", err)
      }
      if err := os.Chmod(tmpExe, 0755); err != nil {
      	_ = os.Remove(tmpExe)
      	return "", err
      }
      if err := os.Rename(tmpExe, currentExe); err != nil {
      	_ = os.Remove(tmpExe)
      	if os.IsPermission(err) {
      		return "", fmt.Errorf("keine Schreibberechtigung für %s (bitte mit 'sudo openlocalcrm update' ausführen): %w", currentExe, err)
      	}
      	return "", fmt.Errorf("failed atomic executable rename: %w", err)
      }
      return currentExe, nil
      ```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/launcher -run "TestApplyStagedFiles_SkipsAllBinaries"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/updater.go internal/launcher/updater_test.go
git commit -m "feat(updater): support atomic Unix inode binary replacement and platform candidate lookup"
```

---

### Task 5: Web GUI Adaptive Layout for macOS & Linux

**Files:**
- Modify: `internal/launcher/ui/index.html`
- Modify: `internal/launcher/ui/app.js`

**Interfaces:**
- Updates UI DOM to reflect `os` and `arch` reported by `/api/preflight`.

- [ ] **Step 1: Update `index.html` markup**

In `internal/launcher/ui/index.html`:
- Change title to `OpenLocalCRM — Setup & Control Center`.
- Add `id="status-wsl-card"` to the WSL status card.
- Add descriptive helper text placeholder `id="preflight-intro-text"`.

- [ ] **Step 2: Update `app.js` dynamic preflight handling**

In `internal/launcher/ui/app.js`:
- In `runPreflightCheck()`:
  - If `status.os !== "windows"`:
    - Hide `document.getElementById('status-wsl-card')` (`style.display = 'none'`).
    - Adjust intro text: `Wir prüfen Ihr System auf Docker und Port-Verfügbarkeit.`.
  - Update header badge:
    ```js
    const badge = document.getElementById('launcher-version-badge');
    if (badge && status.os) {
      const osLabel = status.os === 'darwin' ? 'macOS' : (status.os === 'linux' ? 'Linux' : 'Windows');
      const archLabel = status.arch === 'arm64' ? 'Apple Silicon / ARM' : 'x86_64';
      badge.textContent = `v3.0 ${osLabel} (${archLabel})`;
    }
    ```
  - Platform-adaptive Docker hints:
    - If macOS and Docker stopped: show button `▶️ Docker starten` (calls `/api/system/start-docker`).
    - If Linux and Docker stopped: show `sudo systemctl start docker` or `sudo usermod -aG docker $USER`.

- [ ] **Step 3: Run UI package test**

Run: `go test -v ./internal/launcher/ui`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/launcher/ui/index.html internal/launcher/ui/app.js
git commit -m "feat(ui): adapt preflight DOM dynamically for macOS and Linux"
```

---

### Task 6: Multi-Arch Build Targets & Universal Install Script

**Files:**
- Modify: `Makefile`
- Create: `scripts/install.sh`

**Interfaces:**
- Produces `Makefile` targets:
  - `build-darwin-arm64`
  - `build-darwin-amd64`
  - `build-linux-amd64`
  - `build-linux-arm64`
  - `build-all-launcher`
- Produces `scripts/install.sh` for one-liner install:
  `curl -fsSL https://raw.githubusercontent.com/boreddev1/openlocalcrm/main/scripts/install.sh | bash`

- [ ] **Step 1: Add cross-compilation targets to `Makefile`**

Add targets:
```make
build-darwin-arm64: ## Build macOS Apple Silicon binary
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-darwin-arm64 ./cmd/setup-launcher

build-darwin-amd64: ## Build macOS Intel binary
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-darwin-amd64 ./cmd/setup-launcher

build-linux-amd64: ## Build Linux x86_64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-linux-amd64 ./cmd/setup-launcher

build-linux-arm64: ## Build Linux ARM64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LAUNCHER_LDFLAGS)" -o bin/openlocalcrm-setup-linux-arm64 ./cmd/setup-launcher

build-all-launcher: build-windows build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64
```

- [ ] **Step 2: Create `scripts/install.sh`**

Create `scripts/install.sh`:
- Detect OS via `uname -s` (`Darwin` -> `darwin`, `Linux` -> `linux`).
- Detect Architecture via `uname -m` (`x86_64` -> `amd64`, `arm64`/`aarch64` -> `arm64`).
- Check target directory: `/usr/local/bin` (or `~/.local/bin` if non-root).
- Download binary from GitHub Releases / Raw fallback.
- Set executable permission (`chmod +x`).
- Verify installation by executing `openlocalcrm --version`.

- [ ] **Step 3: Test shell script syntax and dry-run execution**

Run: `bash -n scripts/install.sh`
Expected: PASS (syntax valid)

- [ ] **Step 4: Commit**

```bash
git add Makefile scripts/install.sh
git commit -m "feat(build): add multi-arch build targets and universal install script"
```

---

### Task 7: Full System Build & Cross-Platform Verification

**Files:**
- All modified files across repository.

- [ ] **Step 1: Execute `make build-all-launcher`**

Run: `make build-all-launcher`
Expected: Successfully generates 5 binaries in `bin/`:
- `bin/openlocalcrm-setup.exe`
- `bin/openlocalcrm-setup-darwin-arm64`
- `bin/openlocalcrm-setup-darwin-amd64`
- `bin/openlocalcrm-setup-linux-amd64`
- `bin/openlocalcrm-setup-linux-arm64`

- [ ] **Step 2: Test local macOS binary directly**

Run: `./bin/openlocalcrm-setup-darwin-arm64 --version`
Run: `./bin/openlocalcrm-setup-darwin-arm64 --help`
Run: `./bin/openlocalcrm-setup-darwin-arm64 status`
Expected: CLI output executes cleanly and exits with code 0.

- [ ] **Step 3: Run entire Go test suite**

Run: `go test ./...`
Expected: 100% PASS across all packages.

- [ ] **Step 4: Commit and tag**

```bash
git add Makefile bin/
git commit -m "build: compile cross-platform launcher binaries for macOS and Linux"
```
