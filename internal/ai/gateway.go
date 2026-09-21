package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrUpstreamUnavailable signals that the configured AI backend could not be
// reached or returned an unsuccessful response. Callers must surface this as an
// honest error instead of inventing a response.
var ErrUpstreamUnavailable = errors.New("KI-Dienst nicht erreichbar")

type Provider string

const (
	ProviderOllama    Provider = "ollama"
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
)

type LLMClient interface {
	Generate(ctx context.Context, prompt string, systemInstruction string) (string, error)
}

type GatewayConfig struct {
	DefaultProvider Provider
	OllamaBaseURL   string
	OllamaModel     string // e.g. "gemma2:12b"
	APIKey          string
}

type Gateway struct {
	cfg   GatewayConfig
	guard *Guard
	http  *http.Client
}

func NewGateway(cfg GatewayConfig) *Gateway {
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
	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = ProviderOllama
	}
	timeout := 30 * time.Second
	if tStr := os.Getenv("OLLAMA_TIMEOUT_SECONDS"); tStr != "" {
		if tSec, err := strconv.Atoi(tStr); err == nil && tSec > 0 {
			timeout = time.Duration(tSec) * time.Second
		}
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 2 * time.Second,
		}).DialContext,
	}
	return &Gateway{
		cfg:   cfg,
		guard: NewGuard(),
		http:  &http.Client{Transport: transport, Timeout: timeout},
	}
}

func (g *Gateway) GetConfig() GatewayConfig {
	return g.cfg
}

// Generate sends a prompt to the configured LLM backend with automatic PII sanitization and prompt guards
func (g *Gateway) Generate(ctx context.Context, prompt string, systemInstruction string) (string, error) {
	// 1. Sanitize input prompt and system instructions for PII (Finding #38)
	cleanPrompt := g.guard.SanitizeInput(prompt)
	cleanSystem := g.guard.SanitizeInput(systemInstruction)

	switch g.cfg.DefaultProvider {
	case ProviderOllama:
		return g.callOllama(ctx, cleanPrompt, cleanSystem)
	case ProviderOpenAI:
		return g.callOpenAI(ctx, cleanPrompt, cleanSystem)
	case ProviderAnthropic:
		return "", fmt.Errorf("%w: ai provider 'anthropic' ist noch nicht konfiguriert", ErrUpstreamUnavailable)
	case ProviderGemini:
		return "", fmt.Errorf("%w: ai provider 'gemini' ist noch nicht konfiguriert", ErrUpstreamUnavailable)
	default:
		return "", fmt.Errorf("%w: unbekannter AI-Provider: %s", ErrUpstreamUnavailable, g.cfg.DefaultProvider)
	}
}

func (g *Gateway) callOpenAI(ctx context.Context, prompt string, systemInstruction string) (string, error) {
	baseURL := g.cfg.OllamaBaseURL
	if baseURL == "" || strings.Contains(baseURL, "11434") {
		baseURL = "https://api.openai.com/v1"
	}
	// Finding #30 & #37: Do not silently overwrite OpenAI model if explicitly configured
	model := g.cfg.OllamaModel
	if aiModel := os.Getenv("AI_MODEL"); aiModel != "" {
		model = aiModel
	} else if openAIModel := os.Getenv("OPENAI_MODEL"); openAIModel != "" {
		model = openAIModel
	} else if model == "" || model == "gemma4:12b" {
		model = "gpt-4o-mini"
	}

	messages := []map[string]string{}
	if systemInstruction != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": systemInstruction,
		})
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": prompt,
	})

	reqMap := map[string]any{
		"model":    model,
		"messages": messages,
	}

	reqBody, _ := json.Marshal(reqMap)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("%w: openai request build failed: %v", ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: openai request failed: %v", ErrUpstreamUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: openai backend returned HTTP %d", ErrUpstreamUnavailable, resp.StatusCode)
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("%w: openai response decode failed: %v", ErrUpstreamUnavailable, err)
	}
	if len(res.Choices) > 0 {
		out, err := g.guard.ValidateOutput(res.Choices[0].Message.Content)
		if err != nil {
			return "", fmt.Errorf("%w: openai output validation failed: %v", ErrUpstreamUnavailable, err)
		}
		return out, nil
	}
	return "", fmt.Errorf("%w: openai backend returned no choices", ErrUpstreamUnavailable)
}

func (g *Gateway) callOllama(ctx context.Context, prompt string, systemInstruction string) (string, error) {
	reqMap := map[string]any{
		"model":  g.cfg.OllamaModel,
		"prompt": prompt,
		"system": systemInstruction,
		"stream": false,
	}
	if strings.Contains(strings.ToLower(systemInstruction), "json") {
		reqMap["format"] = "json"
	}

	reqBody, _ := json.Marshal(reqMap)

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.OllamaBaseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("%w: ollama request build failed: %v", ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: ollama request failed: %v", ErrUpstreamUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: ollama backend returned HTTP %d", ErrUpstreamUnavailable, resp.StatusCode)
	}

	var res struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("%w: ollama response decode failed: %v", ErrUpstreamUnavailable, err)
	}

	out, err := g.guard.ValidateOutput(res.Response)
	if err != nil {
		return "", fmt.Errorf("%w: ollama output validation failed: %v", ErrUpstreamUnavailable, err)
	}
	return out, nil
}
