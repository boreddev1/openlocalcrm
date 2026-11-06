package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

type AIProbeRequest struct {
	Provider       string `json:"provider"` // ollama, openai, gemini, anthropic, custom
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

// probeEmbedding validates the embedding backend via the ai gateway in
// pure-constructor mode (no env fallbacks). It never touches
// TranslateHostForDocker: the launcher probes the URL the user actually
// entered. A nil result means the provider needs no embedding probe.
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
	return &EmbeddingProbeResult{
		Success: true,
		Message: fmt.Sprintf("Embedding OK (%d Dimensionen)", ai.EmbeddingDimensions),
		Dims:    ai.EmbeddingDimensions,
	}
}

func ProbeAIConnection(ctx context.Context, req AIProbeRequest) AIProbeResponse {
	start := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		if req.Provider == "ollama" {
			baseURL = "http://localhost:11434"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var res AIProbeResponse

	chatFail := func(msg string) AIProbeResponse {
		return AIProbeResponse{
			Success: false,
			Message: msg,
			Chat: &ChatProbeResult{
				Success:   false,
				Message:   msg,
				LatencyMS: time.Since(start).Milliseconds(),
			},
		}
	}

	if req.Provider == "ollama" {
		probeURL := baseURL + "/api/tags"
		httpReq, err := http.NewRequestWithContext(probeCtx, "GET", probeURL, nil)
		if err != nil {
			return chatFail(fmt.Sprintf("Ungültige URL: %v", err))
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			return chatFail(fmt.Sprintf("Verbindung zu Ollama fehlgeschlagen: %v", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return chatFail(fmt.Sprintf("Ollama antwortete mit Status: %d", resp.StatusCode))
		}

		res = AIProbeResponse{
			Success:   true,
			Message:   fmt.Sprintf("Verbindung zu Ollama erfolgreich hergestellt! (Modell: %s)", req.Model),
			LatencyMS: time.Since(start).Milliseconds(),
		}
		res.Chat = &ChatProbeResult{
			Success:   true,
			Message:   res.Message,
			LatencyMS: time.Since(start).Milliseconds(),
		}
	} else {
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
			return chatFail(fmt.Sprintf("Ungültige Anfrage: %v", err))
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if req.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			return chatFail(fmt.Sprintf("Verbindung fehlgeschlagen: %v", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return chatFail("401 Unauthorized: Ungültiger API-Key")
		}
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusOK {
			return chatFail(fmt.Sprintf("KI-Endpunkt meldete HTTP-Fehler: %d", resp.StatusCode))
		}

		res = AIProbeResponse{
			Success:   true,
			Message:   fmt.Sprintf("Verbindung zum KI-Endpunkt erfolgreich! (Modell: %s)", req.Model),
			LatencyMS: time.Since(start).Milliseconds(),
		}
		res.Chat = &ChatProbeResult{
			Success:   true,
			Message:   res.Message,
			LatencyMS: time.Since(start).Milliseconds(),
		}
	}

	emb := probeEmbedding(probeCtx, req, baseURL)
	res.Embedding = emb
	if res.Success && emb != nil && !emb.Success {
		res.Success = false
		res.Message = emb.Message
	}
	return res
}

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
