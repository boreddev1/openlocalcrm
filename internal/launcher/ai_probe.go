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
