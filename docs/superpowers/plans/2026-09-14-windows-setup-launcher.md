# Windows Setup & Mini-Control-Center Binary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone, CGO-free Windows binary (`bin/openlocalcrm-setup.exe`, Alias: `bin/mavalio-setup.exe`) that guides users through onboarding and setup of OpenLocalCRM (Docker, PostgreSQL 16, Caddy, AI connection), and functions as a persistent System-Tray Mini-Control-Center for container lifecycle and backups.

**Architecture:** A lightweight Go application (`cmd/setup-launcher`) embedding an HTML5/CSS/JS web interface that serves strictly on loopback (`127.0.0.1`), automatically launches the user's default browser, manages Docker Compose via process calls with `CREATE_NO_WINDOW`, and registers a native Windows System-Tray icon via Win32 syscalls without CGO.

**Tech Stack:** Go 1.22+, Win32 Syscalls (`shell32.dll`, `user32.dll`), Go `embed.FS`, Docker Compose CLI, HTML5/CSS3/Vanilla JS.

## Global Constraints
- Target platform: Windows 10/11 x64 (`GOOS=windows GOARCH=amd64`).
- CGO-free: Must build with `CGO_ENABLED=0` without requiring MinGW or external C toolchains.
- Window management: Must be linked with `-ldflags="-H=windowsgui"` and all sub-processes must set `CreationFlags: 0x08000000` (`CREATE_NO_WINDOW`) to eliminate console flashing.
- Loopback isolation: Internal HTTP server binds exclusively to `127.0.0.1`, never `0.0.0.0`.
- Host-gateway translation: AI URLs referencing `localhost` or `127.0.0.1` must automatically be translated to `http://host.docker.internal` for Docker container reachability.

---

### Task 1: Pre-Flight Checker & System Probes

**Files:**
- Create: `internal/launcher/preflight.go`
- Test: `internal/launcher/preflight_test.go`

**Interfaces:**
- Produces:
  ```go
  type SystemStatus struct {
      DockerInstalled bool   `json:"docker_installed"`
      DockerRunning   bool   `json:"docker_running"`
      DockerVersion   string `json:"docker_version"`
      Port80Free      bool   `json:"port_80_free"`
      Port8080Free    bool   `json:"port_8080_free"`
      SuggestedPort   int    `json:"suggested_port"`
      TotalRAMMB      uint64 `json:"total_ram_mb"`
  }
  func RunPreflightCheck(ctx context.Context) SystemStatus
  func CheckPortAvailable(port int) bool
  ```

- [ ] **Step 1: Write the failing test for pre-flight checks**

```go
package launcher

import (
	"context"
	"net"
	"testing"
)

func TestCheckPortAvailable(t *testing.T) {
	// Bind to an arbitrary available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Port should NOT be available while listening
	if CheckPortAvailable(port) {
		t.Errorf("expected port %d to be unavailable while listener is active", port)
	}

	// Close listener, port should become available
	listener.Close()
	if !CheckPortAvailable(port) {
		t.Errorf("expected port %d to be available after listener closed", port)
	}
}

func TestRunPreflightCheck(t *testing.T) {
	status := RunPreflightCheck(context.Background())
	if status.SuggestedPort != 80 && status.SuggestedPort != 8080 {
		t.Errorf("expected suggested port 80 or 8080, got %d", status.SuggestedPort)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestCheckPortAvailable`
Expected: FAIL with compilation error (undefined: CheckPortAvailable)

- [ ] **Step 3: Implement preflight checker**

Create `internal/launcher/preflight.go`:
```go
package launcher

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

type SystemStatus struct {
	DockerInstalled bool   `json:"docker_installed"`
	DockerRunning   bool   `json:"docker_running"`
	DockerVersion   string `json:"docker_version"`
	Port80Free      bool   `json:"port_80_free"`
	Port8080Free    bool   `json:"port_8080_free"`
	SuggestedPort   int    `json:"suggested_port"`
}

func CheckPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func RunPreflightCheck(ctx context.Context) SystemStatus {
	status := SystemStatus{
		Port80Free:   CheckPortAvailable(80),
		Port8080Free: CheckPortAvailable(8080),
	}

	if status.Port80Free {
		status.SuggestedPort = 80
	} else if status.Port8080Free {
		status.SuggestedPort = 8080
	} else {
		status.SuggestedPort = 8000
	}

	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := execCommand(checkCtx, "docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err == nil {
		status.DockerInstalled = true
		status.DockerRunning = true
		status.DockerVersion = strings.TrimSpace(string(out))
	} else {
		// Check if docker CLI exists at all
		if _, lookErr := exec.LookPath("docker"); lookErr == nil {
			status.DockerInstalled = true
			status.DockerRunning = false
		} else {
			status.DockerInstalled = false
			status.DockerRunning = false
		}
	}

	return status
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestCheckPortAvailable`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/preflight.go internal/launcher/preflight_test.go
git commit -m "feat(launcher): implement pre-flight system and port availability checks"
```

---

### Task 2: AI Connection Probe & Host-Gateway Mapping

**Files:**
- Create: `internal/launcher/ai_probe.go`
- Test: `internal/launcher/ai_probe_test.go`

**Interfaces:**
- Produces:
  ```go
  type AIProbeRequest struct {
      Provider string `json:"provider"` // ollama, openai, gemini, anthropic, custom
      BaseURL  string `json:"base_url"`
      APIKey   string `json:"api_key"`
      Model    string `json:"model"`
  }
  type AIProbeResponse struct {
      Success   bool   `json:"success"`
      Message   string `json:"message"`
      LatencyMS int64  `json:"latency_ms"`
  }
  func ProbeAIConnection(ctx context.Context, req AIProbeRequest) AIProbeResponse
  func TranslateHostForDocker(urlStr string) string
  ```

- [ ] **Step 1: Write failing test for AI probe and Docker host translation**

```go
package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranslateHostForDocker(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"http://localhost:11434", "http://host.docker.internal:11434"},
		{"http://127.0.0.1:11434", "http://host.docker.internal:11434"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"http://192.168.1.50:8000", "http://192.168.1.50:8000"},
	}

	for _, c := range cases {
		actual := TranslateHostForDocker(c.input)
		if actual != c.expected {
			t.Errorf("TranslateHostForDocker(%q) = %q; want %q", c.input, actual, c.expected)
		}
	}
}

func TestProbeAIConnection_OllamaSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma2:12b"}]}`))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama",
		BaseURL:  ts.URL,
		Model:    "gemma2:12b",
	})

	if !res.Success {
		t.Fatalf("expected successful probe, got error: %s", res.Message)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestTranslateHostForDocker`
Expected: FAIL with compilation error (undefined: TranslateHostForDocker)

- [ ] **Step 3: Implement AI probe logic**

Create `internal/launcher/ai_probe.go`:
```go
package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func TranslateHostForDocker(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		port := u.Port()
		if port != "" {
			u.Host = "host.docker.internal:" + port
		} else {
			u.Host = "host.docker.internal"
		}
		return u.String()
	}
	return urlStr
}

func ProbeAIConnection(ctx context.Context, req AIProbeRequest) AIProbeResponse {
	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		if req.Provider == "ollama" {
			baseURL = "http://localhost:11434"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	if req.Provider == "ollama" {
		probeURL := baseURL + "/api/tags"
		httpReq, err := http.NewRequestWithContext(probeCtx, "GET", probeURL, nil)
		if err != nil {
			return AIProbeResponse{Success: false, Message: fmt.Sprintf("Ungültige URL: %v", err)}
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			return AIProbeResponse{Success: false, Message: fmt.Sprintf("Verbindung zu Ollama fehlgeschlagen: %v", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return AIProbeResponse{Success: false, Message: fmt.Sprintf("Ollama antwortete mit Status: %d", resp.StatusCode)}
		}

		return AIProbeResponse{
			Success:   true,
			Message:   fmt.Sprintf("Verbindung zu Ollama erfolgreich hergestellt! (Modell: %s)", req.Model),
			LatencyMS: time.Since(start).Milliseconds(),
		}
	}

	// OpenAI-kompatibler Endpunkt (OpenAI, OpenRouter, vLLM, Groq, etc.)
	probeURL := baseURL + "/chat/completions"
	body, _ := json.Marshal(map[string]any{
		"model": req.Model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 5,
	})

	httpReq, err := http.NewRequestWithContext(probeCtx, "POST", probeURL, bytes.NewReader(body))
	if err != nil {
		return AIProbeResponse{Success: false, Message: fmt.Sprintf("Ungültige Anfrage: %v", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return AIProbeResponse{Success: false, Message: fmt.Sprintf("Verbindung fehlgeschlagen: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return AIProbeResponse{Success: false, Message: "401 Unauthorized: Ungültiger API-Key"}
	}
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusOK {
		return AIProbeResponse{Success: false, Message: fmt.Sprintf("KI-Endpunkt meldete HTTP-Fehler: %d", resp.StatusCode)}
	}

	return AIProbeResponse{
		Success:   true,
		Message:   fmt.Sprintf("Verbindung zum KI-Endpunkt erfolgreich! (Modell: %s)", req.Model),
		LatencyMS: time.Since(start).Milliseconds(),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run "TestTranslateHostForDocker|TestProbeAIConnection"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/ai_probe.go internal/launcher/ai_probe_test.go
git commit -m "feat(launcher): implement AI connection probe and host.docker.internal mapping"
```

---

### Task 3: Configuration & Cryptographic Secret Generator

**Files:**
- Create: `internal/launcher/config.go`
- Test: `internal/launcher/config_test.go`

**Interfaces:**
- Produces:
  ```go
  type SetupConfig struct {
      AdminEmail     string `json:"admin_email"`
      AdminPassword  string `json:"admin_password"`
      Port           int    `json:"port"`
      IsDemoMode     bool   `json:"is_demo_mode"`
      AIProvider     string `json:"ai_provider"`
      AIBaseURL      string `json:"ai_base_url"`
      AIAPIKey       string `json:"ai_api_key"`
      AIModel        string `json:"ai_model"`
  }
  func GenerateEnvContent(cfg SetupConfig) (string, error)
  func WriteConfigAndDirectories(baseDir string, cfg SetupConfig) error
  func IsAlreadyInstalled(baseDir string) bool
  ```

- [ ] **Step 1: Write failing test for config generator**

```go
package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEnvContent(t *testing.T) {
	cfg := SetupConfig{
		AdminEmail:    "admin@mavalio.local",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestGenerateEnvContent`
Expected: FAIL with compilation error (undefined: GenerateEnvContent)

- [ ] **Step 3: Implement config writer and secret generator**

Create `internal/launcher/config.go`:
```go
package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func IsAlreadyInstalled(baseDir string) bool {
	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		return true
	}
	return false
}

func GenerateEnvContent(cfg SetupConfig) (string, error) {
	if cfg.Port <= 0 {
		cfg.Port = 80
	}
	if cfg.AdminEmail == "" {
		cfg.AdminEmail = "admin@mavalio.local"
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = randomHex(12)
	}

	dbPassword := randomHex(16)
	connectorToken := randomHex(24)

	aiBaseURL := cfg.AIBaseURL
	if aiBaseURL != "" {
		aiBaseURL = TranslateHostForDocker(aiBaseURL)
	} else if cfg.AIProvider == "ollama" {
		aiBaseURL = "http://host.docker.internal:11434"
	}

	lines := []string{
		"# ==============================================================================",
		"# mavalio CRM - Auto-generated Production Configuration",
		"# ==============================================================================",
		"DOMAIN=localhost",
		fmt.Sprintf("PORT=%d", cfg.Port),
		"LOG_LEVEL=info",
		"",
		"# Administrator Account",
		fmt.Sprintf("INITIAL_ADMIN_EMAIL=%s", cfg.AdminEmail),
		fmt.Sprintf("INITIAL_ADMIN_PASSWORD=%s", cfg.AdminPassword),
		"",
		"# Database (PostgreSQL 16)",
		"DB_HOST=crm-db",
		"DB_PORT=5432",
		"DB_NAME=mavalio",
		"DB_USER=postgres",
		fmt.Sprintf("DB_PASSWORD=%s", dbPassword),
		fmt.Sprintf("DATABASE_URL=postgres://postgres:%s@crm-db:5432/mavalio?sslmode=disable", dbPassword),
		"",
		"# Authentication & Keys",
		"JWT_SECRET_KEY_PATH=/storage/keys/ed25519.key",
		"STORAGE_PATH=/storage",
		"",
		"# Connectors & Webhooks",
		fmt.Sprintf("CONNECTOR_API_TOKEN=%s", connectorToken),
		"",
		"# AI Gateway",
		fmt.Sprintf("AI_PROVIDER=%s", cfg.AIProvider),
		fmt.Sprintf("OLLAMA_BASE_URL=%s", aiBaseURL),
		fmt.Sprintf("OLLAMA_MODEL=%s", cfg.AIModel),
		fmt.Sprintf("AI_API_KEY=%s", cfg.AIAPIKey),
		fmt.Sprintf("AI_BASE_URL=%s", aiBaseURL),
		"",
	}

	if cfg.IsDemoMode {
		lines = append(lines, "DEMO_MODE=true", "")
	}

	return strings.Join(lines, "\n"), nil
}

func WriteConfigAndDirectories(baseDir string, cfg SetupConfig) error {
	content, err := GenerateEnvContent(cfg)
	if err != nil {
		return err
	}

	// Create necessary directories
	dirs := []string{
		filepath.Join(baseDir, "data", "postgres"),
		filepath.Join(baseDir, "data", "storage", "keys"),
		filepath.Join(baseDir, "backups"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed creating directory %s: %w", d, err)
		}
	}

	envPath := filepath.Join(baseDir, ".env")
	return os.WriteFile(envPath, []byte(content), 0600)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run "TestGenerateEnvContent|TestIsAlreadyInstalled"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/config.go internal/launcher/config_test.go
git commit -m "feat(launcher): implement configuration and secret generation engine"
```

---

### Task 4: Docker Engine & Process Orchestrator

**Files:**
- Create: `internal/launcher/engine.go`
- Create: `internal/launcher/engine_windows.go`
- Create: `internal/launcher/engine_other.go`
- Test: `internal/launcher/engine_test.go`

**Interfaces:**
- Produces:
  ```go
  type ContainerInfo struct {
      Name    string `json:"name"`
      Service string `json:"service"`
      State   string `json:"state"`
      Status  string `json:"status"`
      Health  string `json:"health"`
  }
  type Engine struct {
      BaseDir string
  }
  func NewEngine(baseDir string) *Engine
  func (e *Engine) Up(ctx context.Context, logChan chan<- string) error
  func (e *Engine) Down(ctx context.Context) error
  func (e *Engine) Stop(ctx context.Context) error
  func (e *Engine) Restart(ctx context.Context) error
  func (e *Engine) GetContainers(ctx context.Context) ([]ContainerInfo, error)
  func (e *Engine) StreamLogs(ctx context.Context, logChan chan<- string) error
  func (e *Engine) CreateBackup(ctx context.Context) (string, error)
  ```

- [ ] **Step 1: Write failing test for Engine container parsing**

```go
package launcher

import (
	"testing"
)

func TestParseDockerComposePS(t *testing.T) {
	rawJSON := `[
		{"Name":"mavalio-server-1","Service":"crm-server","State":"running","Status":"Up 2 hours (healthy)","Health":"healthy"},
		{"Name":"mavalio-db-1","Service":"crm-db","State":"running","Status":"Up 2 hours (healthy)","Health":"healthy"}
	]`

	containers, err := ParseContainersJSON([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].Service != "crm-server" || containers[0].Health != "healthy" {
		t.Errorf("unexpected container data: %+v", containers[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestParseDockerComposePS`
Expected: FAIL with compilation error (undefined: ParseContainersJSON)

- [ ] **Step 3: Implement Engine & Windows CREATE_NO_WINDOW command builder**

Create `internal/launcher/engine_windows.go`:
```go
//go:build windows

package launcher

import (
	"context"
	"os/exec"
	"syscall"
)

func execCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}
```

Create `internal/launcher/engine_other.go`:
```go
//go:build !windows

package launcher

import (
	"context"
	"os/exec"
)

func execCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
```

Create `internal/launcher/engine.go`:
```go
package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ContainerInfo struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Health  string `json:"health"`
}

func ParseContainersJSON(data []byte) ([]ContainerInfo, error) {
	var list []ContainerInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

type Engine struct {
	BaseDir string
}

func NewEngine(baseDir string) *Engine {
	return &Engine{BaseDir: baseDir}
}

func (e *Engine) Up(ctx context.Context, logChan chan<- string) error {
	cmd := execCommand(ctx, "docker", "compose", "up", "-d")
	cmd.Dir = e.BaseDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		text := scanner.Text()
		if logChan != nil {
			logChan <- text
		}
	}

	return cmd.Wait()
}

func (e *Engine) Stop(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "stop")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

func (e *Engine) Restart(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "restart")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

func (e *Engine) Down(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "down")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

func (e *Engine) GetContainers(ctx context.Context) ([]ContainerInfo, error) {
	cmd := execCommand(ctx, "docker", "compose", "ps", "--format", "json")
	cmd.Dir = e.BaseDir

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return ParseContainersJSON(out)
}

func (e *Engine) CreateBackup(ctx context.Context) (string, error) {
	backupDir := filepath.Join(e.BaseDir, "backups")
	_ = os.MkdirAll(backupDir, 0755)

	filename := fmt.Sprintf("mavalio_backup_%s.sql", time.Now().Format("2006-01-02_150405"))
	outPath := filepath.Join(backupDir, filename)

	file, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	cmd := execCommand(ctx, "docker", "compose", "exec", "-T", "crm-db", "pg_dump", "-U", "postgres", "mavalio")
	cmd.Dir = e.BaseDir
	cmd.Stdout = file

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("pg_dump failed: %w", err)
	}

	return filename, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestParseDockerComposePS`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/engine.go internal/launcher/engine_windows.go internal/launcher/engine_other.go internal/launcher/engine_test.go
git commit -m "feat(launcher): implement Docker Compose engine and Windows command execution"
```

---

### Task 5: Embedded Setup & Control Center Web UI

**Files:**
- Create: `internal/launcher/ui/embed.go`
- Create: `internal/launcher/ui/index.html`
- Create: `internal/launcher/ui/style.css`
- Create: `internal/launcher/ui/app.js`
- Test: `internal/launcher/ui/embed_test.go`

**Interfaces:**
- Produces:
  ```go
  package ui
  var DistFS embed.FS
  func Handler() http.Handler
  ```

- [ ] **Step 1: Write failing test for embedded UI filesystem**

```go
package ui

import (
	"io/fs"
	"testing"
)

func TestUIFilesExist(t *testing.T) {
	requiredFiles := []string{"index.html", "style.css", "app.js"}
	for _, file := range requiredFiles {
		if _, err := fs.Stat(DistFS, file); err != nil {
			t.Errorf("required file %q missing from embedded DistFS: %v", file, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher/ui`
Expected: FAIL (package not found or missing files)

- [ ] **Step 3: Create responsive Slate/Emerald Setup & Control UI**

Create `internal/launcher/ui/embed.go`:
```go
package ui

import (
	"embed"
	"net/http"
)

//go:embed index.html style.css app.js
var DistFS embed.FS

func Handler() http.Handler {
	return http.FileServer(http.FS(DistFS))
}
```

Create `internal/launcher/ui/style.css`: Modern Tailwind/Slate inspired stylesheet matching the CRM branding (dark header, emerald badges, clean card containers, tab navigation, terminal stream box).

Create `internal/launcher/ui/index.html`:
- Step 1: Pre-flight status cards with actionable buttons.
- Step 2: Form for Admin email, password (with generate button), port input, and flexible AI selector with "KI-Verbindung testen" button and live response badge.
- Step 3: Deployment progress bar and SSE live log terminal.
- Step 4: Success card with copyable credentials and "Im Browser öffnen" button.
- Control-Center Dashboard: Status-Ampeln, Service Actions (Start, Stop, Restart), Log-Viewer mit Filter, Backup-Button.

Create `internal/launcher/ui/app.js`: Reactive controller connecting to `/api/status`, `/api/preflight`, `/api/ai/test`, `/api/setup`, and `/api/control/*`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher/ui`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/ui/
git commit -m "feat(launcher): add embedded responsive HTML5/CSS/JS Setup and Control UI"
```

---

### Task 6: HTTP Server & REST API

**Files:**
- Create: `internal/launcher/server.go`
- Test: `internal/launcher/server_test.go`

**Interfaces:**
- Produces:
  ```go
  func NewServer(baseDir string, engine *Engine) http.Handler
  ```

- [ ] **Step 1: Write failing test for server status and preflight endpoints**

```go
package launcher

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerStatusEndpoint(t *testing.T) {
	engine := NewEngine(t.TempDir())
	handler := NewServer(t.TempDir(), engine)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/launcher -run TestServerStatusEndpoint`
Expected: FAIL with compilation error (undefined: NewServer)

- [ ] **Step 3: Implement server routes and handlers**

Create `internal/launcher/server.go`:
- Route `GET /api/status`: Returns `{ "installed": bool, "containers": [...] }`
- Route `POST /api/preflight`: Runs `RunPreflightCheck`
- Route `POST /api/ai/test`: Runs `ProbeAIConnection`
- Route `POST /api/setup`: Accepts `SetupConfig`, writes `.env`, triggers `engine.Up`
- Route `POST /api/control/{action}`: Executes `engine.Start/Stop/Restart`
- Route `POST /api/backup`: Triggers `engine.CreateBackup`
- Route `GET /`: Serves `ui.Handler()`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/launcher -run TestServerStatusEndpoint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/server.go internal/launcher/server_test.go
git commit -m "feat(launcher): implement REST API and SSE endpoints for launcher server"
```

---

### Task 7: Windows Entrypoint, Browser Auto-Launch & System Tray

**Files:**
- Create: `cmd/setup-launcher/main.go`
- Create: `cmd/setup-launcher/tray_windows.go`
- Create: `cmd/setup-launcher/tray_other.go`

**Interfaces:**
- Win32 Syscall tray registration on Windows.
- Platform-independent `openBrowser(url)` helper.

- [ ] **Step 1: Implement platform-isolated System-Tray and Browser Launch**

Create `cmd/setup-launcher/tray_windows.go` with Win32 Syscall NotifyIcon implementation.
Create `cmd/setup-launcher/tray_other.go` with stub for macOS/Linux.
Create `cmd/setup-launcher/main.go`:
- Binds to `127.0.0.1:9099` (or next free port).
- Starts background HTTP server.
- Opens browser tab automatically (`rundll32 url.dll,FileProtocolHandler` or `cmd /c start`).
- Enters Win32 Message Pump / Tray event loop.

- [ ] **Step 2: Run build on macOS to verify compile compatibility**

Run: `go build -o /dev/null ./cmd/setup-launcher`
Expected: SUCCESS (0 exit code)

- [ ] **Step 3: Commit**

```bash
git add cmd/setup-launcher/
git commit -m "feat(launcher): implement main entrypoint, browser launcher, and Win32 tray hooks"
```

---

### Task 8: Makefile Windows Build Targets & Verification

**Files:**
- Modify: `Makefile:20-22`

- [ ] **Step 1: Add Windows build targets to Makefile**

Add:
```makefile
build-windows: ## Build CGO-free Windows Release Binary (no console window)
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s -H=windowsgui" -o bin/mavalio-setup.exe ./cmd/setup-launcher

build-windows-debug: ## Build Windows Debug Binary with visible console
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/mavalio-setup-debug.exe ./cmd/setup-launcher
```

- [ ] **Step 2: Execute `make build-windows` to verify cross-compilation**

Run: `make build-windows`
Expected: `bin/mavalio-setup.exe` created.

- [ ] **Step 3: Verify Windows PE executable header**

Run: `file bin/mavalio-setup.exe`
Expected: `PE32+ executable (GUI) x86-64, for MS Windows`

- [ ] **Step 4: Run full test suite for launcher**

Run: `go test -v ./internal/launcher/...`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "feat(build): add Windows build targets to Makefile"
```

---

## Execution Choice

Two execution options:
1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints.
