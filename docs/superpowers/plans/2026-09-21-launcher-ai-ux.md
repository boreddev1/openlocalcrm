# Launcher-KI-UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** KI-Bereich des Installers: echte Modell-Datalists, Mistral/Nebius-Presets (auf openai-kompatibel gemappt), ein Test-Button mit zwei getrennten Statuszeilen, ehrliche Hinweiszeile.

**Architecture:** Der Probe-Response bekommt ein additives `chat`-Sub-Objekt (spiegelt `embedding`), damit die UI zwei unabhängige Statuszeilen rendern kann. Ein neuer Endpoint `/api/ai/models` liefert echte Modelllisten (Ollama `/api/tags`, OpenAI-kompatibel `GET /models`) mit ehrlichem Fallback auf Freitext. Presets bleiben UI-Mapping (keine Backend-Provider-Änderung).

**Tech Stack:** Go 1.26 (chi, net/http, httptest), Vanilla-JS-Launcher-UI (embedded).

**Spec:** `docs/superpowers/specs/2026-09-21-launcher-ai-ux-design.md` — binding.

## Global Constraints

- Zero fake success: Modellabruf/Probe schlagen ehrlich fehl; Datalist-Fehlschlag degradiert auf Freitext, niemals erfundene Modellliste.
- Presets `mistral`/`nebius` mappen UI-seitig auf `openai` (Chat im Gateway unterstützt nur ollama/openai); NIEMALS `AI_PROVIDER=mistral` schreiben.
- Probe-Response bleibt rückwärtskompatibel: neue Felder kommen dazu, nichts wird entfernt.
- `TranslateHostForDocker` bleibt aus dem Models-Endpoint raus (Host-Perspektive).
- Kein Caching (RAM-Budget); Timeout 10 s für /api/ai/models.
- Tests hermetisch (httptest); UI live verifiziert (Controller). Conventional Commits; `make check` grün.

---

### Task A: Probe-`chat`-Subobjekt + `/api/ai/models` Endpoint

**Files:**
- Modify: `internal/launcher/ai_probe.go` (AIProbeResponse, ProbeAIConnection, neuer `ListAIModels`)
- Modify: `internal/launcher/server.go` (Route neben `/api/ai/test`, Handler)
- Test: `internal/launcher/ai_probe_test.go`, `internal/launcher/server_test.go`

**Interfaces:**
- Consumes: bestehendes `ProbeAIConnection`, `AIProbeResponse`
- Produces: `AIProbeResponse.Chat *ChatProbeResult` (`{success, message, latency_ms}`) — immer gesetzt; `ListAIModels(ctx, ListAIModelsQuery{Provider, BaseURL, APIKey}) ListAIModelsResponse` mit `{success, models []string, message}`; Route `GET /api/ai/models?provider=&base_url=&api_key=`

- [ ] **Step 1: Failing Tests schreiben** in `internal/launcher/ai_probe_test.go`:

```go
func TestProbeAIConnection_ChatSubResultAlwaysPresent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
			return
		}
		vec := make([]string, 1024)
		for i := range vec {
			vec[i] = "0.1"
		}
		_, _ = w.Write([]byte(`{"embedding":[` + strings.Join(vec, ",") + `]}`))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
		EmbeddingModel: "qwen3-embedding:0.6b",
	})
	if res.Chat == nil || !res.Chat.Success {
		t.Fatalf("expected chat sub-result ok, got %+v", res.Chat)
	}
	if res.Embedding == nil || !res.Embedding.Success {
		t.Fatalf("expected embedding sub-result ok, got %+v", res.Embedding)
	}
	if !res.Success {
		t.Fatalf("expected overall success")
	}
}

func TestListAIModels_OllamaSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"},{"name":"qwen3-embedding:0.6b"}]}`))
	}))
	defer ts.Close()

	res := ListAIModels(context.Background(), ListAIModelsQuery{Provider: "ollama", BaseURL: ts.URL})
	if !res.Success || len(res.Models) != 2 || res.Models[0] != "gemma4:12b" {
		t.Fatalf("expected 2 models from ollama, got %+v", res)
	}
}

func TestListAIModels_OpenAICompatibleSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"mistral-large-latest"},{"id":"mistral-embed"}]}`))
	}))
	defer ts.Close()

	res := ListAIModels(context.Background(), ListAIModelsQuery{Provider: "openai", BaseURL: ts.URL, APIKey: "k"})
	if !res.Success || len(res.Models) != 2 || res.Models[1] != "mistral-embed" {
		t.Fatalf("expected 2 models from openai-compatible, got %+v", res)
	}
}

func TestListAIModels_HonestFailures(t *testing.T) {
	// Unreachable
	res := ListAIModels(context.Background(), ListAIModelsQuery{Provider: "ollama", BaseURL: "http://127.0.0.1:1"})
	if res.Success || res.Message == "" {
		t.Fatalf("expected honest failure, got %+v", res)
	}
	// Bad status
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer ts.Close()
	res = ListAIModels(context.Background(), ListAIModelsQuery{Provider: "openai", BaseURL: ts.URL})
	if res.Success || !strings.Contains(res.Message, "403") {
		t.Fatalf("expected status message, got %+v", res)
	}
	// Unparseable
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer ts2.Close()
	res = ListAIModels(context.Background(), ListAIModelsQuery{Provider: "openai", BaseURL: ts2.URL})
	if res.Success {
		t.Fatalf("expected failure on unparseable body, got %+v", res)
	}
	// Invalid provider
	res = ListAIModels(context.Background(), ListAIModelsQuery{Provider: "mistral"})
	if res.Success {
		t.Fatalf("expected failure for non ollama/openai provider")
	}
}
```

In `server_test.go` einen Route-Test ergänzen (Muster der bestehenden `/api/ai/test`-Tests): `GET /api/ai/models?provider=ollama&base_url=<ts.URL>` → 200 mit `{"success":true,"models":[...]}`; ohne `provider` → 400.

- [ ] **Step 2: Tests ausführen, Fehlschlag verifizieren**

Run: `go test -v -run 'TestProbeAIConnection_ChatSubResult|TestListAIModels' ./internal/launcher/`
Expected: FAIL — `unknown field Chat` / `undefined: ListAIModels`.

- [ ] **Step 3: Implementierung**:

`ai_probe.go` — Response-Strukturen erweitern:

```go
type ChatProbeResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

type AIProbeResponse struct {
	Success   bool                  `json:"success"`
	Message   string                `json:"message"`
	LatencyMS int64                 `json:"latency_ms"`
	Chat      *ChatProbeResult      `json:"chat,omitempty"`
	Embedding *EmbeddingProbeResult `json:"embedding,omitempty"`
}
```

In `ProbeAIConnection` jeden Chat-Ausgang als `res.Chat = &ChatProbeResult{...}` setzen (Success, Message, LatencyMS aus dem jeweiligen Pfad — Early-Returns inklusive; die bestehende Overall-Logik bleibt unverändert).

Neuer Typ + Funktion (gleiche Datei):

```go
type ListAIModelsQuery struct {
	Provider string
	BaseURL  string
	APIKey   string
}

type ListAIModelsResponse struct {
	Success bool     `json:"success"`
	Models  []string `json:"models"`
	Message string   `json:"message,omitempty"`
}

func ListAIModels(ctx context.Context, q ListAIModelsQuery) ListAIModelsResponse {
	switch q.Provider {
	case "ollama", "openai":
	default:
		return ListAIModelsResponse{Success: false, Message: "Provider muss ollama oder openai sein"}
	}
	baseURL := strings.TrimRight(q.BaseURL, "/")
	if baseURL == "" {
		if q.Provider == "ollama" {
			baseURL = "http://localhost:11434"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	path := "/api/tags"
	if q.Provider == "openai" {
		path = "/models"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return ListAIModelsResponse{Success: false, Message: fmt.Sprintf("Ungültige URL: %v", err)}
	}
	if q.Provider == "openai" && q.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+q.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ListAIModelsResponse{Success: false, Message: fmt.Sprintf("Verbindung fehlgeschlagen: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ListAIModelsResponse{Success: false, Message: fmt.Sprintf("Endpunkt antwortete mit Status %d", resp.StatusCode)}
	}
	var raw struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ListAIModelsResponse{Success: false, Message: fmt.Sprintf("Antwort unlesbar: %v", err)}
	}
	models := make([]string, 0, len(raw.Models)+len(raw.Data))
	for _, m := range raw.Models {
		if m.Name != "" {
			models = append(models, m.Name)
		}
	}
	for _, m := range raw.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return ListAIModelsResponse{Success: true, Models: models}
}
```

`server.go` — Route neben `/api/ai/test` registrieren und Handler:

```go
mux.HandleFunc("/api/ai/models", s.handleAIModels)

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, `{"error":"provider is required"}`, http.StatusBadRequest)
		return
	}
	res := ListAIModels(r.Context(), ListAIModelsQuery{
		Provider: provider,
		BaseURL:  r.URL.Query().Get("base_url"),
		APIKey:   r.URL.Query().Get("api_key"),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
```

- [ ] **Step 4: Tests ausführen, Pass verifizieren**

Run: `go test -v ./internal/launcher/ && go test ./...`
Expected: PASS (bestehende Probe-Tests müssen weiter grün sein — `chat`-Feld ist additiv).

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/ai_probe.go internal/launcher/server.go internal/launcher/ai_probe_test.go internal/launcher/server_test.go
git commit -m "feat(launcher): expose chat sub-result in probe and real model listing endpoint"
```

---

### Task B: Launcher-UI (Datalists, Presets, zwei Statuszeilen, Hinweiszeile)

**Files:**
- Modify: `internal/launcher/ui/index.html` (KI-Box: Provider-Optionen, Datalists, Statuszeilen-Container, Hinweiszeile)
- Modify: `internal/launcher/ui/app.js` (`effectiveAIProvider`, `onAIProviderChange`, `loadAIModels`/`fillDatalist`, `testAIConnection`-Rendering, `startSetup`-Payload)

**Interfaces:**
- Consumes: `GET /api/ai/models` (Task A), `AIProbeResponse.chat`/`.embedding` (Task A), bestehende Feld-IDs

- [ ] **Step 1: `index.html`** — Presets, Datalists, Statuszeilen, Hinweiszeile:

```html
<select id="ai-provider" onchange="onAIProviderChange()">
  <option value="ollama">🖥️ Lokales Ollama (DSGVO 100 % lokal)</option>
  <option value="openai">🌐 OpenAI / OpenAI-kompatibel (OpenRouter, Groq, vLLM, etc.)</option>
  <option value="mistral">⚡ Mistral AI (OpenAI-kompatibel)</option>
  <option value="nebius">☁️ Nebius AI Studio (OpenAI-kompatibel)</option>
  <option value="none">🚫 Keine KI (vorerst deaktiviert)</option>
</select>
```

Modell- und Embedding-Feld als Datalist-Inputs (bestehende `form-group`-Struktur beibehalten):

```html
<input type="text" id="ai-model" list="ai-model-options" value="gemma4:12b">
<datalist id="ai-model-options"></datalist>
<input type="text" id="ai-embedding-model" list="ai-embedding-model-options" placeholder="mistral-embed (beliebiges 1024-dim Modell)">
<datalist id="ai-embedding-model-options"></datalist>
<p class="hint">Benötigt wird ein 1024-dimensionales Modell (DB-Spalte vector(1024)); bei Mismatch startet crm-server nicht. Anthropic/DeepSeek bieten keine Embedding-API an.</p>
```

(`.hint` an das vorhandene Kleintext-Styling angleichen, z. B. gleiche Klassen wie Nachbar-Hinweise.)

Statuszeilen: bestehendes `ai-probe-status` ersetzen durch zwei Container:

```html
<div id="ai-chat-status"></div>
<div id="ai-embedding-status"></div>
```

- [ ] **Step 2: `app.js`** — Provider-Mapping und Datalists:

```js
function effectiveAIProvider() {
  const v = document.getElementById('ai-provider').value;
  if (v === 'mistral' || v === 'nebius') return 'openai';
  return v;
}

const AI_PRESET_URLS = {
  mistral: 'https://api.mistral.ai/v1',
  nebius: 'https://api.studio.nebius.ai/v1'
};
```

`onAIProviderChange` — `mistral`/`nebius` laufen über den OpenAI-Zweig (Key-Gruppe sichtbar), mit URL-Prefill, leerem Modell- und Embedding-Feld; am Ende `loadAIModels()` aufrufen. Chat-Default `gemma4:12b` (ollama) bleibt.

```js
async function loadAIModels() {
  const provider = document.getElementById('ai-provider').value;
  if (provider === 'none') return;
  try {
    const res = await fetch('/api/ai/models?' + new URLSearchParams({
      provider: effectiveAIProvider(),
      base_url: document.getElementById('ai-url').value,
      api_key: document.getElementById('ai-key').value
    }));
    const data = await res.json();
    fillDatalist('ai-model-options', data);
    fillDatalist('ai-embedding-model-options', data);
  } catch (err) {
    fillDatalist('ai-model-options', null);
    fillDatalist('ai-embedding-model-options', null);
  }
}

function fillDatalist(id, data) {
  const dl = document.getElementById(id);
  if (!dl) return;
  dl.innerHTML = '';
  if (data && data.success && Array.isArray(data.models)) {
    for (const m of data.models) {
      const opt = document.createElement('option');
      opt.value = m;
      dl.appendChild(opt);
    }
  }
}
```

URL-Feld `change`-Event → `loadAIModels()` (Listener im Init-Abschnitt registrieren).

- [ ] **Step 3: `app.js`** — `testAIConnection` nutzt `effectiveAIProvider()` und rendert zwei Statuszeilen:

```js
const payload = {
  provider: effectiveAIProvider(),
  base_url: document.getElementById('ai-url').value,
  api_key: document.getElementById('ai-key').value,
  model: document.getElementById('ai-model').value,
  embedding_model: document.getElementById('ai-embedding-model').value
};
// … fetch wie bisher …
const chatEl = document.getElementById('ai-chat-status');
const embEl = document.getElementById('ai-embedding-status');
if (data.chat) {
  chatEl.textContent = (data.chat.success ? '✅ Chat: ' : '❌ Chat: ') + data.chat.message + (data.chat.success ? ' (' + data.chat.latency_ms + ' ms)' : '');
  chatEl.style.color = data.chat.success ? '#34d399' : '#f87171';
}
if (data.embedding) {
  embEl.textContent = (data.embedding.success ? '✅ Embedding: ' : '❌ Embedding: ') + data.embedding.message;
  embEl.style.color = data.embedding.success ? '#34d399' : '#f87171';
} else {
  embEl.textContent = '';
}
```

(Netzwerkfehler `catch` → beide Zeilen ehrlich rot mit `err.message`.)

- [ ] **Step 4: `app.js`** — `startSetup`-Payload: `ai_provider: effectiveAIProvider()` (Mapping gilt für Setup identisch), restliche Felder unverändert inkl. `ai_embedding_model`.

- [ ] **Step 5: Verifikation + Commit**

```bash
node --check internal/launcher/ui/app.js
go test ./internal/launcher/ui/ ./internal/launcher/
git add internal/launcher/ui/index.html internal/launcher/ui/app.js
git commit -m "feat(launcher-ui): real model datalists, mistral/nebius presets, and split probe status lines"
```

- [ ] **Step 6 (Controller): Live-Verifikation** — Launcher neu bauen/starten, durchklicken:
  1. Ollama: Datalists füllen mit lokalen Modellen; Test-Button → zwei grüne Statuszeilen (Chat-Latenz, Embedding 1024 dims).
  2. Preset „Mistral AI": URL-Prefill `https://api.mistral.ai/v1`, Key-Gruppe sichtbar, Payload-Provider = `openai` (DevTools-Network-Check).
  3. Preset „Nebius AI Studio": URL-Prefill `https://api.studio.nebius.ai/v1`.
  4. Fehlerfälle: nicht erreichbare URL → beide Zeilen rot mit konkreter Ursache; `.hint`-Zeile sichtbar.
