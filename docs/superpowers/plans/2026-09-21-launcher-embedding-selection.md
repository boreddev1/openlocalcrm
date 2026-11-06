# Launcher Embedding-Auswahl Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Der Setup-Launcher ermöglicht die Auswahl des Embedding-Modells (nur wo sinnvoll), validiert es live im Verbindungstest und rekonfiguriert bestehende Installationen ehrlich.

**Architecture:** Der Launcher-Probe ruft das echte `internal/ai`-Gateway über einen neuen reinen Konstruktormodus (`GatewayConfig.DisableEnvFallback`) auf — kein Nachbau der Embedding-Logik. Konfiguration fließt provider-scoped durch `.env`-Generierung, Recovery-Whitelist und Settings-Export/Import; bestehende Compose-Dateien werden surgisch gepatcht. Chat-Auswahl bleibt unverändert.

**Tech Stack:** Go 1.26 (chi, net/http), sqlc-freie Launcher-Ebene, Embedded UI (Vanilla JS), httptest.

**Spec:** `docs/superpowers/specs/2026-09-21-launcher-embedding-selection-design.md` (Rev. 4) — binding.

## Global Constraints

- Zero fake success: externe Abhängigkeiten schlagen ehrlich fehl (HTTP 502/503 bzw. Probe-❌), niemals `err == nil` mit erfundenen Daten.
- Kein Launcher-Env-Wert darf in den Probe sickern (`DisableEnvFallback` = reiner Konstruktor).
- Chat-Probe bleibt eigener HTTP-Client — niemals `NewGateway.Generate` (callOpenAI liest `AI_MODEL`/`OPENAI_MODEL` am Flag vorbei und nutzt `OllamaBaseURL`).
- Probe-Timeouts: 30 s (Chat- und Embedding-Teil), Startcheck in `cmd/server/main.go` 30 s fix (nie `OLLAMA_TIMEOUT_SECONDS`).
- Auflösungsregel provider-scoped: Formularwert → `AI_EMBEDDING_MODEL` → `OLLAMA_EMBEDDING_MODEL` **nur bei Ollama** → leer.
- `.env.example` bleibt unverändert; `AIProbeRequest`-Tag `embedding_model`, Setup-Tag `ai_embedding_model`.
- Migrations-Symlink intakt; Conventional Commits; Coverage-Gate `./scripts/coverage.sh --min 50.0 --diff 50.0`; `make check` grün.
- Alle Tests hermetisch (httptest, keine Live-Aufrufe); Live-Verifikation des UI durch den Controller (Playwright MCP).

---

### Task 1: Gateway `DisableEnvFallback` + Startcheck-Timeout

**Files:**
- Modify: `internal/ai/gateway.go` (GatewayConfig struct ~Zeile 30, NewGateway ~Zeile 100–135)
- Modify: `cmd/server/main.go:161` (Context-Cap)
- Test: `internal/ai/gateway_test.go`

**Interfaces:**
- Consumes: bestehendes `NewGateway(cfg GatewayConfig)`, `GatewayConfig{DefaultProvider, OllamaBaseURL, OllamaModel, EmbeddingModel, APIKey}`
- Produces: `GatewayConfig.DisableEnvFallback bool` — wenn `true`: keine env-Backfills (`AI_EMBEDDING_MODEL`, `OLLAMA_EMBEDDING_MODEL`, `AI_API_KEY`, `OLLAMA_MODEL`, `OLLAMA_BASE_URL`) und Client-Timeout fix 30 s (`OLLAMA_TIMEOUT_SECONDS` ignoriert). Strukturelle Defaults (`http://localhost:11434` für Ollama) bleiben. `EmbeddingModel` bekommt im Pure-Modus keinen Default.

- [ ] **Step 1: Failing Tests schreiben** in `internal/ai/gateway_test.go`:

```go
func TestNewGatewayDisableEnvFallbackIgnoresEnv(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "env-embedding-model")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "env-alias-model")
	t.Setenv("AI_API_KEY", "env-api-key")
	t.Setenv("OLLAMA_MODEL", "env-chat-model")
	t.Setenv("OLLAMA_BASE_URL", "http://env-host:9999")

	gw := ai.NewGateway(ai.GatewayConfig{DefaultProvider: ai.ProviderOllama, DisableEnvFallback: true})
	cfg := gw.GetConfig()
	if cfg.EmbeddingModel != "" {
		t.Errorf("expected empty EmbeddingModel, got %q", cfg.EmbeddingModel)
	}
	if cfg.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", cfg.APIKey)
	}
	if cfg.OllamaModel != "" {
		t.Errorf("expected empty OllamaModel, got %q", cfg.OllamaModel)
	}
	if cfg.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("expected structural Ollama default, got %q", cfg.OllamaBaseURL)
	}
}

func TestNewGatewayDisableEnvFallbackForcesDefaultTimeout(t *testing.T) {
	t.Setenv("OLLAMA_TIMEOUT_SECONDS", "1")
	body := `{"embedding":[` + strings.TrimSuffix(strings.Repeat("1.0,", 1024), ",") + `]}`
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer slow.Close()

	withFlag := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama, OllamaBaseURL: slow.URL,
		EmbeddingModel: "test-model", DisableEnvFallback: true,
	})
	if _, err := withFlag.GenerateEmbedding(context.Background(), "text"); err != nil {
		t.Fatalf("expected success with forced 30s timeout, got: %v", err)
	}

	withoutFlag := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama, OllamaBaseURL: slow.URL,
		EmbeddingModel: "test-model",
	})
	if _, err := withoutFlag.GenerateEmbedding(context.Background(), "text"); !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected timeout error (1s from env) without flag, got: %v", err)
	}
}
```

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestNewGatewayDisableEnvFallback' ./internal/ai/`
Expected: FAIL — `unknown field DisableEnvFallback` (Feld existiert noch nicht).

- [ ] **Step 3: Implementierung** in `internal/ai/gateway.go`:

```go
type GatewayConfig struct {
	DefaultProvider     Provider
	OllamaBaseURL       string
	OllamaModel         string
	EmbeddingModel      string
	APIKey              string
	DisableEnvFallback  bool
}
```

In `NewGateway` die env-Backfills und den Timeout blockieren:

```go
func NewGateway(cfg GatewayConfig) *Gateway {
	if !cfg.DisableEnvFallback {
		if cfg.OllamaBaseURL == "" {
			if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
				cfg.OllamaBaseURL = envURL
			} else {
				cfg.OllamaBaseURL = "http://localhost:11434"
			}
		}
		if cfg.OllamaModel == "" {
			if envModel := os.Getenv("OLLAMA_MODEL"); envModel != "" {
				cfg.OllamaModel = envModel
			} else {
				cfg.OllamaModel = "gemma4:12b"
			}
		}
		if cfg.EmbeddingModel == "" {
			if envModel := os.Getenv("AI_EMBEDDING_MODEL"); envModel != "" {
				cfg.EmbeddingModel = envModel
			} else if envModel := os.Getenv("OLLAMA_EMBEDDING_MODEL"); envModel != "" {
				cfg.EmbeddingModel = envModel
			} else if cfg.DefaultProvider == ProviderOllama {
				cfg.EmbeddingModel = DefaultOllamaEmbeddingModel
			}
		}
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("AI_API_KEY")
		}
	}
	timeout := 30 * time.Second
	if !cfg.DisableEnvFallback {
		if tStr := os.Getenv("OLLAMA_TIMEOUT_SECONDS"); tStr != "" {
			if tSec, err := strconv.Atoi(tStr); err == nil && tSec > 0 {
				timeout = time.Duration(tSec) * time.Second
			}
		}
	}
	// … rest unchanged (transport, Gateway construction)
}
```

In `cmd/server/main.go:161` den Cap auf 30 s fix setzen (nie der Generierungs-Timeout):

```go
embeddingCtx, cancelEmbeddingCheck := context.WithTimeout(ctx, 30*time.Second)
```

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v -run 'TestNewGatewayDisableEnvFallback' ./internal/ai/ && go test ./...`
Expected: PASS (ganze Suite).

- [ ] **Step 5: Commit**

```bash
git add internal/ai/gateway.go internal/ai/gateway_test.go cmd/server/main.go
git commit -m "feat(ai): pure-constructor mode for gateway env isolation and 30s startup-check cap"
```

---

### Task 2: Launcher-Probe mit Embedding-Validierung

**Files:**
- Modify: `internal/launcher/ai_probe.go`
- Test: `internal/launcher/ai_probe_test.go`

**Interfaces:**
- Consumes: `ai.GatewayConfig.DisableEnvFallback` (Task 1), `ai.DefaultOllamaEmbeddingModel`, `ai.EmbeddingDimensions`, `ai.ErrEmbeddingDimensionMismatch`, `ai.ErrEmbeddingModelNotConfigured`, `ai.ValidateEmbeddingModel(ctx) error`
- Produces: `AIProbeRequest.EmbeddingModel` (JSON `embedding_model`), `AIProbeResponse.Embedding *EmbeddingProbeResult` (`{success, message, dims}`), Overall-`Success` = Chat OK UND Embedding OK

- [ ] **Step 1: Failing Tests schreiben** in `internal/launcher/ai_probe_test.go`:

```go
func TestProbeAIConnection_OllamaEmbeddingOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
		case "/api/embeddings":
			vec := make([]string, 1024)
			for i := range vec {
				vec[i] = "0.1"
			}
			_, _ = w.Write([]byte(`{"embedding":[` + strings.Join(vec, ",") + `]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
		EmbeddingModel: "qwen3-embedding:0.6b",
	})
	if !res.Success {
		t.Fatalf("expected overall success, got: %s", res.Message)
	}
	if res.Embedding == nil || !res.Embedding.Success || res.Embedding.Dims != ai.EmbeddingDimensions {
		t.Fatalf("expected embedding ok with dims=%d, got %+v", ai.EmbeddingDimensions, res.Embedding)
	}
}

func TestProbeAIConnection_OllamaEmbeddingDimensionMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
			return
		}
		vec := make([]string, 512)
		for i := range vec {
			vec[i] = "0.1"
		}
		_, _ = w.Write([]byte(`{"embedding":[` + strings.Join(vec, ",") + `]}`))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
		EmbeddingModel: "tiny-model",
	})
	if res.Success {
		t.Fatalf("expected overall failure on dimension mismatch")
	}
	if !strings.Contains(res.Message, "crm-server wird mit dieser Konfiguration nicht starten") {
		t.Fatalf("expected fatal-consequence wording, got: %s", res.Message)
	}
}

func TestProbeAIConnection_OpenAIEmptyModelNoHTTPCall(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path == "/chat/completions" {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "openai", BaseURL: ts.URL, APIKey: "k", Model: "gpt-4o-mini",
	})
	if res.Success {
		t.Fatalf("expected failure with empty embedding model for openai provider")
	}
	if !strings.Contains(res.Message, "AI_EMBEDDING_MODEL") {
		t.Fatalf("expected honest config error, got: %s", res.Message)
	}
	if int32(2) < atomic.LoadInt32(&calls) { // chat call may probe; embeddings must not add calls
		t.Fatalf("unexpected extra calls: %d", calls)
	}
}

func TestProbeAIConnection_IgnoresLauncherEnv(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "env-model")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "qwen3-embedding:0.6b") {
			http.Error(w, `{"error":"wrong model"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		vec := make([]string, 1024)
		for i := range vec {
			vec[i] = "0.1"
		}
		_, _ = w.Write([]byte(`{"embedding":[` + strings.Join(vec, ",") + `]}`))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
	})
	if !res.Success {
		t.Fatalf("expected probe to use explicit default model, not env, got: %s", res.Message)
	}
}

func TestProbeAIConnection_URLMappingUsesEnteredPort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/embeddings":
			vec := make([]string, 1024)
			for i := range vec {
				vec[i] = "0.1"
			}
			_, _ = w.Write([]byte(`{"embedding":[` + strings.Join(vec, ",") + `]}`))
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL + "/", Model: "gemma4:12b",
		EmbeddingModel: "qwen3-embedding:0.6b",
	})
	if !res.Success {
		t.Fatalf("expected trailing slash to be trimmed and URL to be used, got: %s", res.Message)
	}
}
```

Bestehende Chat-Tests (`TestProbeAIConnection_OllamaSuccess`, `TestProbeAIConnection_OpenAISuccess`) anpassen: Fake-Ollama muss zusätzlich `/api/embeddings` mit 1024-dim-Vektor bedienen (oder `EmbeddingModel` setzen) — sonst rot.

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestProbeAIConnection' ./internal/launcher/`
Expected: FAIL — `unknown field EmbeddingModel` / fehlendes `Embedding`-Feld.

- [ ] **Step 3: Implementierung** in `internal/launcher/ai_probe.go`:

```go
type AIProbeRequest struct {
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

type EmbeddingProbeResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Dims    int    `json:"dims,omitempty"`
}

type AIProbeResponse struct {
	Success   bool                  `json:"success"`
	Message   string                `json:"message"`
	LatencyMS int64                 `json:"latency_ms"`
	Embedding *EmbeddingProbeResult `json:"embedding,omitempty"`
}
```

In `ProbeAIConnection`: Chat-Client und Probe-Context auf 30 s (Zeilen 48/59), Chat-Teil unverändert; danach:

```go
func probeEmbedding(ctx context.Context, req AIProbeRequest, enteredURL string) *EmbeddingProbeResult {
	if req.Provider == "none" {
		return nil
	}
	provider := ai.Provider(req.Provider)
	switch provider {
	case ai.ProviderOllama, ai.ProviderOpenAI, ai.ProviderMistral, ai.ProviderNebius:
	default:
		return &EmbeddingProbeResult{Success: true, Message: "Embedding-Prüfung für Provider nicht erforderlich"}
	}
	embModel := strings.TrimSpace(req.EmbeddingModel)
	if embModel == "" {
		if provider == ai.ProviderOllama {
			embModel = ai.DefaultOllamaEmbeddingModel
		} else {
			return &EmbeddingProbeResult{Success: false, Message: "Embedding-Modell ist nicht konfiguriert (AI_EMBEDDING_MODEL)"}
		}
	}
	cfg := ai.GatewayConfig{DefaultProvider: provider, APIKey: req.APIKey, EmbeddingModel: embModel, DisableEnvFallback: true}
	if provider == ai.ProviderOllama {
		cfg.OllamaBaseURL = strings.TrimRight(enteredURL, "/")
	} else {
		cfg.AIBaseURL = strings.TrimRight(enteredURL, "/")
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ai.NewGateway(cfg).ValidateEmbeddingModel(pctx); err != nil {
		msg := err.Error()
		if errors.Is(err, ai.ErrEmbeddingDimensionMismatch) {
			msg = fmt.Sprintf("%s — crm-server wird mit dieser Konfiguration nicht starten (Resize-Migration nötig)", err.Error())
		}
		return &EmbeddingProbeResult{Success: false, Message: msg}
	}
	return &EmbeddingProbeResult{Success: true, Message: fmt.Sprintf("Embedding OK (%d Dimensionen)", ai.EmbeddingDimensions), Dims: ai.EmbeddingDimensions}
}
```

In `ProbeAIConnection` am Ende (nach dem Chat-Ergebnis):

```go
emb := probeEmbedding(probeCtx, req, baseURL)
res.Embedding = emb
if res.Success && emb != nil && !emb.Success {
	res.Success = false
	res.Message = emb.Message
}
```

Hinweis: der eingegebene `baseURL` wird getrimmt weitergegeben (`TranslateHostForDocker` bleibt aus dem Probe-Pfad raus).

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v ./internal/launcher/ && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/ai_probe.go internal/launcher/ai_probe_test.go
git commit -m "feat(launcher): validate embedding model in connection probe via ai gateway"
```

---

### Task 3: Provider-scoped Konfigurationsverdrahtung

**Files:**
- Modify: `internal/launcher/config.go` (SetupConfig ~Zeile 20, ReadExistingConfig ~Zeile 70, Recovery-SetupConfig ~Zeile 138–147, GenerateEnvContentWithExisting ~Zeile 198–290)
- Modify: `internal/launcher/engine.go:86–107` (RecoverEnvFromDocker-Whitelist)
- Test: `internal/launcher/config_test.go`, `internal/launcher/engine_test.go`

**Interfaces:**
- Consumes: `SetupConfig`
- Produces: `SetupConfig.AIEmbeddingModel` (JSON `ai_embedding_model` — fließt automatisch in `/api/status` über `existing_config`), Helper `resolveEmbeddingModel(provider, formValue, aiModel, ollamaModel string) string`, `.env`-Zeilen `AI_EMBEDDING_MODEL`/`OLLAMA_EMBEDDING_MODEL`

- [ ] **Step 1: Failing Tests schreiben** in `internal/launcher/config_test.go`:

```go
func TestGenerateEnvContent_EmbeddingModelProviderScoped(t *testing.T) {
	// Ollama: Formularwert gewinnt
	cfg := SetupConfig{AIProvider: "ollama", AIModel: "gemma4:12b", AIEmbeddingModel: "qwen3-embedding:0.6b"}
	content, err := GenerateEnvContent(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "AI_EMBEDDING_MODEL=qwen3-embedding:0.6b") {
		t.Errorf("expected AI_EMBEDDING_MODEL line, got:\n%s", content)
	}
	if !strings.Contains(content, "OLLAMA_EMBEDDING_MODEL=qwen3-embedding:0.6b") {
		t.Errorf("expected OLLAMA alias line, got:\n%s", content)
	}

	// OpenAI: leeres Feld -> leerer AI-Key, KEINE Alias-Zeile (kein qwen3-Leak)
	cfg = SetupConfig{AIProvider: "openai", AIModel: "gpt-4o-mini"}
	content, err = GenerateEnvContent(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "AI_EMBEDDING_MODEL=\n") {
		t.Errorf("expected empty AI_EMBEDDING_MODEL line, got:\n%s", content)
	}
	if strings.Contains(content, "qwen3-embedding") {
		t.Errorf("openai must never inherit the ollama alias, got:\n%s", content)
	}

	// Bestandsinstallation: nur OLLAMA_EMBEDDING_MODEL gesetzt (Rewrite erhält Alias)
	cfg = SetupConfig{AIProvider: "ollama", AIModel: "gemma4:12b"}
	content, err = GenerateEnvContentWithExisting(cfg, map[string]string{"OLLAMA_EMBEDDING_MODEL": "qwen3-embedding:0.6b"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "AI_EMBEDDING_MODEL=qwen3-embedding:0.6b") {
		t.Errorf("expected alias preserved into AI key on rewrite, got:\n%s", content)
	}

	// none: keine Embedding-Zeilen
	cfg = SetupConfig{AIProvider: "none"}
	content, err = GenerateEnvContent(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "EMBEDDING_MODEL") {
		t.Errorf("none provider must not write embedding lines, got:\n%s", content)
	}
}
```

In `engine_test.go` (RecoverEnvFromDocker-Pfad) eine Assertion ergänzen, dass die Container-Whitelist `AI_EMBEDDING_MODEL` und `OLLAMA_EMBEDDING_MODEL` durchreicht (Muster der bestehenden Container-Env-Tests folgen).

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestGenerateEnvContent_EmbeddingModelProviderScoped' ./internal/launcher/`
Expected: FAIL — `unknown field AIEmbeddingModel`.

- [ ] **Step 3: Implementierung**:

`config.go` — SetupConfig-Feld:

```go
AIEmbeddingModel string `json:"ai_embedding_model"`
```

Provider-scoped Helper (package launcher):

```go
// resolveEmbeddingModel resolves the embedding model provider-scoped: form
// value, then AI_EMBEDDING_MODEL; the OLLAMA_EMBEDDING_MODEL alias is only
// inherited for ollama (its container value often comes from the compose
// default, not a user choice).
func resolveEmbeddingModel(provider, formValue, aiModel, ollamaModel string) string {
	if v := strings.TrimSpace(formValue); v != "" {
		return v
	}
	if v := strings.TrimSpace(aiModel); v != "" {
		return v
	}
	if strings.EqualFold(provider, "ollama") {
		if v := strings.TrimSpace(ollamaModel); v != "" {
			return v
		}
	}
	return ""
}
```

`GenerateEnvContentWithExisting` (nach der `OLLAMA_MODEL`-Zeile):

```go
resolvedEmbedding := resolveEmbeddingModel(cfg.AIProvider, cfg.AIEmbeddingModel, existing["AI_EMBEDDING_MODEL"], existing["OLLAMA_EMBEDDING_MODEL"])
if !strings.EqualFold(cfg.AIProvider, "none") {
	lines = append(lines, fmt.Sprintf("AI_EMBEDDING_MODEL=%s", resolvedEmbedding))
	if resolvedEmbedding != "" {
		lines = append(lines, fmt.Sprintf("OLLAMA_EMBEDDING_MODEL=%s", resolvedEmbedding))
	}
}
```

`ReadExistingConfig` (nach `cfg.AIProvider`):

```go
cfg.AIEmbeddingModel = resolveEmbeddingModel(cfg.AIProvider, envMap["AI_EMBEDDING_MODEL"], envMap["AI_EMBEDDING_MODEL"], envMap["OLLAMA_EMBEDDING_MODEL"])
```

Recovery-SetupConfig in `DetectExistingInstallation` (~Zeile 143):

```go
AIEmbeddingModel: resolveEmbeddingModel(recovered["AI_PROVIDER"], recovered["AI_EMBEDDING_MODEL"], recovered["AI_EMBEDDING_MODEL"], recovered["OLLAMA_EMBEDDING_MODEL"]),
```

`engine.go` Whitelist (nach dem `AI_BASE_URL`-Case):

```go
case "AI_EMBEDDING_MODEL":
	env["AI_EMBEDDING_MODEL"] = v
case "OLLAMA_EMBEDDING_MODEL":
	env["OLLAMA_EMBEDDING_MODEL"] = v
```

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v ./internal/launcher/ && go test ./...`
Expected: PASS (inkl. `/api/status`-Feld über `existing_config`).

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/config.go internal/launcher/engine.go internal/launcher/config_test.go internal/launcher/engine_test.go
git commit -m "feat(launcher): provider-scoped embedding model config wiring and recovery"
```

---

### Task 4: Settings-Export/Import mit EmbeddingModel

**Files:**
- Modify: `internal/launcher/settings.go` (AISettings ~Zeile 12, Export ~Zeile 142, ValidateSettingsValues ~Zeile 164, ImportSettingsToEnv ~Zeile 235–243)
- Test: `internal/launcher/settings_test.go`

**Interfaces:**
- Consumes: `resolveEmbeddingModel` (Task 3)
- Produces: `AISettings.EmbeddingModel string json:"embedding_model"`, Export-JSON-Feld `ai.embedding_model`, Import → `SetupConfig.AIEmbeddingModel`

- [ ] **Step 1: Failing Tests schreiben** in `internal/launcher/settings_test.go`:

```go
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
```

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestSettingsExportImportEmbeddingModel' ./internal/launcher/`
Expected: FAIL — `unknown field EmbeddingModel`.

- [ ] **Step 3: Implementierung** in `internal/launcher/settings.go`:

```go
type AISettings struct {
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url,omitempty"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	HasAPIKey      bool   `json:"has_api_key"`
}
```

Export baut das Feld mit der provider-scoped Auflösung (AI gewinnt):

```go
EmbeddingModel: resolveEmbeddingModel(aiProvider, envMap["AI_EMBEDDING_MODEL"], envMap["AI_EMBEDDING_MODEL"], envMap["OLLAMA_EMBEDDING_MODEL"]),
```

`ImportSettingsToEnv` (settings.go:235 ff.):

```go
cp.AIEmbeddingModel = s.AI.EmbeddingModel
```

`ValidateSettingsValues` (settings.go:164 ff.):

```go
if err := check("ai.embedding_model", s.AI.EmbeddingModel); err != nil {
	return err
}
```

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v ./internal/launcher/ && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/settings.go internal/launcher/settings_test.go
git commit -m "feat(launcher): embed embedding model in settings export/import and validation"
```

---

### Task 5: Compose-Defaults leeren + Bestands-Compose patchen

**Files:**
- Modify: `internal/launcher/templates.go` (defaultDockerCompose Server-Block ~Zeile 44–48, neue Patch-Funktion nach `EnsureComposeAndCaddyFiles` ~Zeile 155)
- Modify: `docker-compose.yml:38`
- Modify: `internal/launcher/config.go` (`WriteConfigAndDirectoriesWithEnv` ~Zeile 302 — Patch-Choke-Point)
- Test: `internal/launcher/templates_test.go`

**Interfaces:**
- Consumes: bestehende `EnsureComposeAndCaddyFiles`, `WriteConfigAndDirectoriesWithEnv`
- Produces: `PatchComposeEmbeddingDefaults(path string) error` — ersetzt idempotent NUR Embedding-Zeilen mit hartkodiertem `qwen3`-Default durch die leere-Default-Form; fehlende Zeilen bleiben fehlend

- [ ] **Step 1: Failing Tests schreiben** in `internal/launcher/templates_test.go`:

```go
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
```

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestPatchComposeEmbeddingDefaults|TestDefaultDockerComposeHasEmptyEmbeddingDefaults' ./internal/launcher/`
Expected: FAIL — `undefined: PatchComposeEmbeddingDefaults`.

- [ ] **Step 3: Implementierung**:

`docker-compose.yml:38` ändern:

```yaml
      - OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-}
```

`templates.go` — Server-Block des `defaultDockerCompose` um die beiden Zeilen ergänzen (identische Form wie oben) und die Patch-Funktion:

```go
// PatchComposeEmbeddingDefaults replaces embedding env lines that hardcode a
// qwen3 compose default with the empty-default form. Missing lines stay
// missing — absence is the safe state (the gateway then falls back to its own
// provider-scoped default or the honest config error).
func PatchComposeEmbeddingDefaults(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		if strings.Contains(line, "OLLAMA_EMBEDDING_MODEL=") && strings.Contains(line, ":-qwen3-embedding:0.6b}") {
			lines[i] = strings.Replace(line, ":-qwen3-embedding:0.6b}", ":-}", 1)
			changed = true
		}
		if strings.Contains(line, "AI_EMBEDDING_MODEL=") && strings.Contains(line, ":-qwen3-embedding:0.6b}") {
			lines[i] = strings.Replace(line, ":-qwen3-embedding:0.6b}", ":-}", 1)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}
```

Aufruf als Choke-Point am Ende von `WriteConfigAndDirectoriesWithEnv` (config.go:302) nach dem `.env`-Schreiben:

```go
_ = PatchComposeEmbeddingDefaults(filepath.Join(baseDir, "docker-compose.yml"))
```

(Fehler nur loggen, nicht die Konfiguration scheitern lassen — die Compose-Datei kann von Hand gepflegt sein; fehlende Datei ist ein No-op.)

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v ./internal/launcher/ && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/templates.go internal/launcher/config.go internal/launcher/templates_test.go docker-compose.yml
git commit -m "fix(launcher): empty embedding compose defaults and patch existing installs (A9)"
```

---

### Task 6: Launcher-UI (kein Unit-Test-Zyklus — Live-Verifikation)

**Files:**
- Modify: `internal/launcher/ui/index.html` (KI-Box ~Zeile 170–195: `ai-model`-Default `gemma2:12b` → `gemma4:12b`, neues Feld)
- Modify: `internal/launcher/ui/app.js` (`onAIProviderChange` ~Zeile 384, `testAIConnection` ~Zeile 407, `startSetup` ~Zeile 440, `editExistingConfig` ~Zeile 59, `loadSettingsFromFile` ~Zeile 1189)
- Test: kein Frontend-Test-Framework für den Launcher — Verifikation live durch den Controller (Playwright MCP, siehe unten). `ui/embed_test.go` bleibt unverändert (Datei-Existenz).

**Interfaces:**
- Consumes: `AIProbeRequest.embedding_model` (Task 2), `/api/status`-Feld `ai_embedding_model` (Task 3), Settings-JSON `ai.embedding_model` (Task 4)

- [ ] **Step 1: `index.html`** — Chat-Default korrigieren und Embedding-Feld in der KI-Box ergänzen:

```html
<input type="text" id="ai-model" value="gemma4:12b">
<div>
  <label for="ai-embedding-model">Embedding-Modell (semantische Suche)</label>
  <input type="text" id="ai-embedding-model" placeholder="mistral-embed (beliebiges 1024-dim Modell)">
</div>
```

- [ ] **Step 2: `app.js` — `onAIProviderChange`** (Chat-Defaults unverändert, Embedding je Provider):

```js
const embeddingInput = document.getElementById('ai-embedding-model');
if (provider === 'none') {
  box.classList.add('hidden');
  embeddingInput.value = '';
} else if (provider === 'ollama') {
  box.classList.remove('hidden');
  keyGroup.classList.add('hidden');
  urlInput.value = 'http://localhost:11434';
  modelInput.value = 'gemma4:12b';
  embeddingInput.value = 'qwen3-embedding:0.6b';
} else {
  box.classList.remove('hidden');
  keyGroup.classList.remove('hidden');
  urlInput.value = 'https://api.openai.com/v1';
  modelInput.value = 'gpt-4o-mini';
  embeddingInput.value = '';
}
```

- [ ] **Step 3: `app.js` — Payloads**: `testAIConnection` ergänzt `embedding_model: document.getElementById('ai-embedding-model').value`; `startSetup` ergänzt `ai_embedding_model: document.getElementById('ai-embedding-model').value`.

- [ ] **Step 4: `app.js` — Reihenfolge beim Laden gespeicherter Werte**: in `editExistingConfig` und `loadSettingsFromFile` NACH dem `onAIProviderChange()`-Aufruf (und nach `ai-model`) das gespeicherte Embedding setzen — sonst überschreibt der Provider-Default den Importwert:

```js
// editExistingConfig, nach dem ai_model-Block:
if (existingSystemConfig.ai_embedding_model) {
  document.getElementById('ai-embedding-model').value = existingSystemConfig.ai_embedding_model;
}
// loadSettingsFromFile, nach onAIProviderChange() und ai.model-Zuweisung:
if (settings.ai && settings.ai.embedding_model) {
  document.getElementById('ai-embedding-model').value = settings.ai.embedding_model;
}
```

- [ ] **Step 5: Frontend-Checks + Commit**

```bash
cd web && pnpm lint && pnpm typecheck && cd ..   # Launcher-UI liegt nicht in web/, aber Repo-Gate muss grün bleiben
go test ./...
git add internal/launcher/ui/index.html internal/launcher/ui/app.js
git commit -m "feat(launcher-ui): embedding model selection with provider-aware defaults and honest probe"
```

- [ ] **Step 6 (Controller): Live-Verifikation mit Playwright MCP** (Invariant #6):
  1. `go run ./cmd/setup-launcher` starten, UI öffnen, mit lokalem Ollama (`qwen3-embedding:0.6b`) den ✅-Fall durchklicken.
  2. Mismatch-Fall: Embedding-Feld auf ein 512-dim-Testmodell → ❌ mit „crm-server wird nicht starten"-Text.
  3. Bestandsinstallation mit nur `OLLAMA_EMBEDDING_MODEL` in `.env` → Rewrite erhält das Modell.
  4. Provider `none` → Feld leer, keine Embedding-Zeilen im erzeugten `.env`.

---

## Final Verification

1. `make check` grün (gofmt, vet, web lint/typecheck, Tests, Lizenzen).
2. `./scripts/coverage.sh --min 50.0 --diff 50.0`.
3. `ls -la migrations` — Symlink intakt.
4. Round-5-Sweep: keine neuen Fake-Success-Pfade (Probe, .env-Generierung, Recovery, Compose-Patch).
