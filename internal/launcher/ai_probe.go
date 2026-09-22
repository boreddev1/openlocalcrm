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

type AIProbeResponse struct {
	Success   bool                  `json:"success"`
	Message   string                `json:"message"`
	LatencyMS int64                 `json:"latency_ms"`
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

		res = AIProbeResponse{
			Success:   true,
			Message:   fmt.Sprintf("Verbindung zu Ollama erfolgreich hergestellt! (Modell: %s)", req.Model),
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

		res = AIProbeResponse{
			Success:   true,
			Message:   fmt.Sprintf("Verbindung zum KI-Endpunkt erfolgreich! (Modell: %s)", req.Model),
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
