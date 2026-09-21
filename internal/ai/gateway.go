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

// ErrEmbeddingDimensionMismatch signals that the embedding backend returned a
// vector whose dimensions do not match the fixed pgvector column width. It is a
// configuration error, not a transient upstream failure, and must never be
// silently truncated or padded.
var ErrEmbeddingDimensionMismatch = errors.New("Embedding-Dimension passt nicht zum Schema")

// ErrEmbeddingsUnsupported signals that the configured provider has no
// embeddings API at all (e.g. Anthropic, DeepSeek). Callers get this instead of
// an invented vector.
var ErrEmbeddingsUnsupported = errors.New("Provider bietet keine Embedding-API an")

// ErrEmbeddingModelNotConfigured signals an honest configuration gap: the
// selected provider supports embeddings but no model was configured. No model
// is ever guessed.
var ErrEmbeddingModelNotConfigured = errors.New("Embedding-Modell ist nicht konfiguriert")

// EmbeddingDimensions is the fixed width of the knowledge_base_articles.embedding
// pgvector column. A model whose output differs requires an explicit migration.
const EmbeddingDimensions = 1024

// DefaultOllamaEmbeddingModel is the fallback embedding model for the local
// Ollama provider (1024 dimensions, verified with qwen3-embedding:0.6b).
const DefaultOllamaEmbeddingModel = "qwen3-embedding:0.6b"

type Provider string

const (
	ProviderOllama    Provider = "ollama"
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
	ProviderMistral   Provider = "mistral"
	ProviderNebius    Provider = "nebius"
	ProviderDeepSeek  Provider = "deepseek"
)

type LLMClient interface {
	Generate(ctx context.Context, prompt string, systemInstruction string) (string, error)
}

type GatewayConfig struct {
	DefaultProvider Provider
	OllamaBaseURL   string
	// AIBaseURL overrides the embeddings base URL for OpenAI-compatible
	// providers (OpenAI, Mistral, Nebius, ...). Resolved from AI_BASE_URL.
	AIBaseURL   string
	OllamaModel string // e.g. "gemma2:12b"
	// EmbeddingModel is the embedding model used for semantic search. Resolved
	// from config, then AI_EMBEDDING_MODEL, then OLLAMA_EMBEDDING_MODEL, then
	// the per-provider default. OpenAI-compatible providers have no default:
	// an unconfigured model is an honest config error, never a guess.
	EmbeddingModel string
	APIKey         string
}

type Gateway struct {
	cfg   GatewayConfig
	guard *Guard
	http  *http.Client
}

func NewGateway(cfg GatewayConfig) *Gateway {
	// Provider resolution comes first: the embedding-model fallback depends on it.
	if cfg.DefaultProvider == "" {
		if envProvider := os.Getenv("AI_PROVIDER"); envProvider != "" {
			cfg.DefaultProvider = Provider(envProvider)
		} else {
			cfg.DefaultProvider = ProviderOllama
		}
	}
	if cfg.OllamaBaseURL == "" {
		if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
			cfg.OllamaBaseURL = envURL
		} else {
			cfg.OllamaBaseURL = "http://localhost:11434"
		}
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = os.Getenv("AI_BASE_URL")
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
			// Only the local Ollama provider has a safe default. OpenAI-compatible
			// providers must be configured explicitly; nothing is guessed.
			cfg.EmbeddingModel = DefaultOllamaEmbeddingModel
		}
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("AI_API_KEY")
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

// GenerateEmbedding returns a real embedding vector for the given text from the
// configured provider. Every transport, status, or decode failure is wrapped with
// ErrUpstreamUnavailable. A vector whose length differs from EmbeddingDimensions
// yields ErrEmbeddingDimensionMismatch; the vector is never silently truncated,
// padded, or invented. Providers without an embeddings API (Anthropic, DeepSeek,
// unknown providers) yield ErrEmbeddingsUnsupported.
func (g *Gateway) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	switch g.cfg.DefaultProvider {
	case ProviderOllama:
		return g.embedOllama(ctx, text)
	case ProviderOpenAI, ProviderMistral, ProviderNebius:
		return g.embedOpenAICompatible(ctx, text)
	case ProviderAnthropic, ProviderDeepSeek:
		return nil, fmt.Errorf("%w: Provider '%s' bietet keine Embedding-API an", ErrEmbeddingsUnsupported, g.cfg.DefaultProvider)
	default:
		return nil, fmt.Errorf("%w: unbekannter AI-Provider '%s' bietet keine Embedding-API an", ErrEmbeddingsUnsupported, g.cfg.DefaultProvider)
	}
}

// ValidateEmbeddingModel probes the embedding backend once and reports whether it
// is reachable and returns vectors of the schema's fixed dimension. It is intended
// for a startup self-check.
func (g *Gateway) ValidateEmbeddingModel(ctx context.Context) error {
	_, err := g.GenerateEmbedding(ctx, "dimension-check")
	return err
}

// ValidateEmbeddingModelFromEnv builds a gateway from environment configuration
// and probes the embedding backend. It lets cmd/server perform the startup check
// using the exact same model resolution as the runtime gateway.
func ValidateEmbeddingModelFromEnv(ctx context.Context) error {
	return NewGateway(GatewayConfig{}).ValidateEmbeddingModel(ctx)
}

func (g *Gateway) checkEmbeddingDimensions(vec []float32) error {
	if len(vec) != EmbeddingDimensions {
		return fmt.Errorf("%w: Modell %q liefert %d Dimensionen, erwartet %d",
			ErrEmbeddingDimensionMismatch, g.cfg.EmbeddingModel, len(vec), EmbeddingDimensions)
	}
	return nil
}

// embeddingsBaseURL resolves the embeddings endpoint base URL for
// OpenAI-compatible providers: explicit config, then AI_BASE_URL, then the
// per-provider default.
func (g *Gateway) embeddingsBaseURL() string {
	if g.cfg.AIBaseURL != "" {
		return strings.TrimRight(g.cfg.AIBaseURL, "/")
	}
	switch g.cfg.DefaultProvider {
	case ProviderMistral:
		return "https://api.mistral.ai/v1"
	case ProviderNebius:
		return "https://api.studio.nebius.ai/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

func (g *Gateway) embedOllama(ctx context.Context, text string) ([]float32, error) {
	reqMap := map[string]any{
		"model":  g.cfg.EmbeddingModel,
		"prompt": text,
	}
	reqBody, _ := json.Marshal(reqMap)

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.OllamaBaseURL+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("%w: ollama embedding request build failed: %v", ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: ollama embedding request failed: %v", ErrUpstreamUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: ollama embedding backend returned HTTP %d", ErrUpstreamUnavailable, resp.StatusCode)
	}

	// /api/embeddings returns {"embedding":[...]}; the newer /api/embed returns
	// {"embeddings":[[...]]}. Accept both without changing the target endpoint.
	var res struct {
		Embedding  []float32   `json:"embedding"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("%w: ollama embedding decode failed: %v", ErrUpstreamUnavailable, err)
	}
	vec := res.Embedding
	if len(vec) == 0 && len(res.Embeddings) > 0 {
		vec = res.Embeddings[0]
	}
	if err := g.checkEmbeddingDimensions(vec); err != nil {
		return nil, err
	}
	return vec, nil
}

// embedOpenAICompatible sends one shared OpenAI-compatible request
// (POST {base}/embeddings with {"model","input"}) used by OpenAI, Mistral AI,
// Nebius AI Studio and any AI_BASE_URL override.
func (g *Gateway) embedOpenAICompatible(ctx context.Context, text string) ([]float32, error) {
	if g.cfg.EmbeddingModel == "" {
		return nil, fmt.Errorf("%w: Für Provider '%s' muss AI_EMBEDDING_MODEL gesetzt sein (kein Standard-Modell wird erraten)",
			ErrEmbeddingModelNotConfigured, g.cfg.DefaultProvider)
	}
	baseURL := g.embeddingsBaseURL()

	reqMap := map[string]any{
		"model": g.cfg.EmbeddingModel,
		"input": text,
	}
	reqBody, _ := json.Marshal(reqMap)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("%w: embedding request build failed: %v", ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: embedding request failed: %v", ErrUpstreamUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s embedding backend returned HTTP %d", ErrUpstreamUnavailable, g.cfg.DefaultProvider, resp.StatusCode)
	}

	var res struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("%w: embedding decode failed: %v", ErrUpstreamUnavailable, err)
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("%w: %s embedding backend returned no data", ErrUpstreamUnavailable, g.cfg.DefaultProvider)
	}
	vec := res.Data[0].Embedding
	if err := g.checkEmbeddingDimensions(vec); err != nil {
		return nil, err
	}
	return vec, nil
}
