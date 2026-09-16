# Design Specification: Universal Cross-Platform GUI & CLI Installer for macOS, Linux, and Windows

**Date:** 2026-09-16  
**Status:** In Review (Self-Reviewed)  
**Author:** Pair Programming (Antigravity & User)

---

## 1. Executive Summary & Goals

The OpenLocalCRM setup launcher currently provides a browser-based GUI and self-updater focused primarily on Windows environments (`openlocalcrm-setup.exe`).

To support developers, Mac desktop users, and system administrators deploying OpenLocalCRM on cloud servers (Hetzner, AWS, Ubuntu/Debian/Fedora) or macOS workstations:
1. **Identical Web GUI on macOS & Linux:** The same setup wizard and control dashboard available at `http://127.0.0.1:9099`, automatically opening the system's default browser (`open` on macOS, `xdg-open` on Linux).
2. **Built-in CLI Subcommand Interface:** When executed with arguments in a terminal or in headless environments without a graphical display, the binary acts directly as a CLI tool (`openlocalcrm status`, `openlocalcrm install`, `openlocalcrm update`, `openlocalcrm backup`, `openlocalcrm reset`), eliminating the need to open a browser.
3. **OS-Adaptive Preflight:** Dynamically hides Windows-specific WSL2 requirements on macOS and Linux, checking for macOS Docker Desktop/OrbStack/Colima or Linux native Docker Engine and user group permissions.
4. **One-Liner Installation Script:** `curl -fsSL https://raw.githubusercontent.com/boreddev1/openlocalcrm/main/scripts/install.sh | bash` that auto-detects architecture and platform.

---

## 2. Architecture: All-in-One Binary

The launcher binary functions as a dual-mode application:
- **No arguments:** Checks display environment. If a desktop GUI environment is available, starts the local HTTP daemon on `:9099` and launches the default web browser (`open` on macOS, `rundll32` on Windows, `xdg-open` on Linux).
- **Headless Server Detection:** On Linux when `$DISPLAY` and `$WAYLAND_DISPLAY` are empty (e.g., SSH session on a cloud VPS), it does **not** attempt `xdg-open` (which would fail or print errors). Instead, it logs the direct URL (`http://<ip>:9099`) with a note for SSH port-forwarding, or instructs the user to use the CLI subcommands (`openlocalcrm --help`).
- **Subcommand provided:** Executes the CLI command directly in the terminal and exits cleanly without launching a background web server (unless running `openlocalcrm gui`).

```mermaid
graph TD
    A[Start: openlocalcrm / openlocalcrm-setup] --> B{Subcommand passed?}
    B -- No --> C{Desktop Display Available?}
    C -- Yes --> D[Start HTTP Daemon :9099 & Open Browser]
    C -- No / Headless --> E[Print Headless Guidance & Daemon URL or CLI Help]
    D --> F[Web GUI: Setup Wizard / Control Center]
    
    B -- Yes --> G[CLI Subcommand Dispatcher]
    G -- status --> H[Print Stack & Container Health Table]
    G -- install --> I[Non-interactive or Terminal Setup]
    G -- start/stop/restart --> J[Control Docker Compose Stack]
    G -- update --> K[GitHub Self-Update via CLI]
    G -- backup --> L[Create / List / Restore SQL Database Dump]
    G -- reset --> M[Factory Reset Stack & Volumes]
    G -- admin --> N[Reset or Bootstrap Admin Password]
    G -- help / --version --> O[Print Usage Information]
```

---

## 3. CLI Subcommand Specifications

### 3.1. `openlocalcrm status [--json]`
- Checks Docker daemon connectivity.
- Lists all CRM containers (name, status, health).
- Prints active local URLs: CRM Web App URL (`http://localhost:<port>`), Launcher Web GUI URL (`http://localhost:9099`).
- Exits with `0` if healthy, `1` if stopped or degraded.
- If `--json` is supplied, returns structured JSON.

### 3.2. `openlocalcrm install [flags]`
- Flags:
  - `--port <int>` (Default: `80`, fallback `8080`)
  - `--admin-email <email>` (Default: `admin@openlocalcrm.local`)
  - `--admin-password <password>` (If omitted, generates a cryptographically secure 16-character password)
  - `--ai-provider <ollama|openai|none>` (Default: `ollama`)
  - `--ai-url <url>` (e.g. `http://localhost:11434`)
  - `--ai-model <model>` (e.g. `gemma2:9b`)
  - `--ai-key <key>` (for OpenAI/external providers)
  - `--demo` (Enables Demo Mode with synthetic data)
  - `-y`, `--yes`, `--non-interactive` (Runs without interactive prompts)
  - `--dir <path>` (Explicit project directory; defaults to `ResolveProjectBaseDir()`)
- Actions:
  - Runs pre-flight checks (Docker engine, port availability).
  - Writes `.env`, `docker-compose.yml`, and `Caddyfile`.
  - Executes `docker compose up -d --remove-orphans`.
  - Displays generated admin credentials and access URL in a formatted ASCII box.

### 3.3. `openlocalcrm start` / `stop` / `restart` / `down`
- Runs `docker compose up -d --remove-orphans`, `docker compose stop`, `docker compose restart`, or `docker compose down`.
- Outputs container state changes to stdout.

### 3.4. `openlocalcrm update`
- Queries GitHub Commits API for updates.
- If updates exist:
  - Runs automatic safety database backup.
  - Downloads and applies repository file updates (respecting `.env` and `backups/` blacklist).
  - Skips binary files in `bin/` during recursive walk to prevent `ETXTBSY` file-busy errors.
  - Rebuilds containers (`docker compose up -d --build --remove-orphans`).
  - Swaps executable (atomic rename on Unix, `.old` delay swap on Windows).
  - If executable is in `/usr/local/bin` and requires root permissions, provides an actionable message (`sudo openlocalcrm update`).

### 3.5. `openlocalcrm backup [create | list | restore <file>]`
- `backup create`: Creates a timestamped `.sql` dump in `backups/` and copies to persistent backup directory.
- `backup list`: Lists all existing `.sql` backup files with size, date, and schema version.
- `backup restore <filename>`: Prompts for confirmation (unless `-y` passed) and restores database cleanly.

### 3.6. `openlocalcrm reset [-f, --force]`
- Prompts for explicit confirmation ("Are you sure you want to delete all CRM data and volumes? [y/N]").
- If stdin is not a TTY and `-f`/`--force` is not passed, aborts with code `1`.
- If confirmed or `-f` passed: stops containers, removes all volumes (`docker compose down -v`), wipes legacy containers/volumes, removes `.env`.

### 3.7. `openlocalcrm admin password <email> <new-password>`
- Direct CLI command to bootstrap or reset administrator credentials without opening the web browser.

---

## 4. Platform-Specific Adaptations

### 4.1. Base Directory Resolution (`ResolveProjectBaseDir`)
When installed globally into `/usr/local/bin/openlocalcrm` or `~/.local/bin/openlocalcrm`, the launcher must not attempt to create project files in `/usr/local/bin`:
1. Check `OPENLOCALCRM_DIR` environment variable.
2. Check `--dir` CLI flag if provided.
3. Check current working directory (`.`) if `docker-compose.yml` or `.env` exists.
4. Check executable directory and its parent directory (if portable / run inside repo clone).
5. On Windows: check `LOCALAPPDATA/openlocalcrm`, `C:\openlocalcrm`, etc.
6. On Unix (macOS & Linux): check `$HOME/.openlocalcrm`. If installing anew and not in a project folder, default to `$HOME/.openlocalcrm`.

### 4.2. Preflight Checks & Privileged Ports
- **WSL2:** On `runtime.GOOS != "windows"`, WSL check returns `wsl_installed: true` with status message `WSL not required on macOS/Linux`.
- **Port 80 Check on Unix:** Ports below 1024 require root privileges for standard `net.Listen`. If `net.Listen` returns `bind: permission denied` on macOS/Linux, the preflight check probes via `net.DialTimeout("tcp", "127.0.0.1:80", 150*time.Millisecond)`. If connection is refused, port 80 is not occupied and Docker (which runs as root) can bind to it.
- **Docker Auto-Start:**
  - Windows: Starts `Docker Desktop.exe` via PowerShell.
  - macOS: Runs `open -a Docker` (or OrbStack if installed).
  - Linux: Detects `systemctl`; provides actionable command `sudo systemctl start docker` or `sudo usermod -aG docker $USER` if permission denied.
- **Web GUI:**
  - Exposes `os` and `arch` in `/api/preflight`.
  - Hides `#status-wsl-card` when `os !== "windows"`.
  - Dynamically updates header badge (`v3.0 macOS`, `v3.0 Linux`, `v3.0 Windows`).

### 4.3. Unix Executable Inode Replacement
- On macOS and Linux, executing binaries cannot be opened for writing (`ETXTBSY`), but standard Unix allows unlinking and renaming open inodes:
  - Write new binary to `currentExe + ".tmp"`.
  - Set permissions `0755` via `os.Chmod`.
  - Atomically replace via `os.Rename(currentExe + ".tmp", currentExe)`.

---

## 5. Build Targets & Distribution

### 5.1. `Makefile` Multi-Arch Targets
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

### 5.2. One-Liner Script: `scripts/install.sh`
- Detects `uname -s` (`Darwin` -> `darwin`, `Linux` -> `linux`).
- Detects `uname -m` (`x86_64` -> `amd64`, `arm64`/`aarch64` -> `arm64`).
- Installs to `/usr/local/bin/openlocalcrm` if writable or via `sudo`; otherwise falls back to `~/.local/bin/openlocalcrm`.
- Sets execute permissions (`chmod +x`).
- Verifies installation by running `openlocalcrm --version`.

---

## 6. Verification Plan

1. **Unit & Integration Tests:**
   - Test CLI command argument routing (`status`, `install`, `backup`, `reset`, `admin`, `help`).
   - Test preflight OS-detection on Darwin/Linux (assert WSL is bypassed, port 80 check handles non-root bind restrictions).
   - Test `ResolveProjectBaseDir()` across different platforms and install locations.
2. **Build Validation:**
   - Cross-compile for `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.
   - Run compiled binary locally on macOS (`./bin/openlocalcrm-setup-darwin-arm64 status`).
3. **One-Liner Verification:**
   - Test `scripts/install.sh` locally with dry-run mode.
