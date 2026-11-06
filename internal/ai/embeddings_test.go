package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func dimensionVector(n int) []float32 {
	vec := make([]float32, n)
	for i := range vec {
		vec[i] = float32(i%7) * 0.01
	}
	return vec
}

func fakeEmbeddingServer(t *testing.T, body map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func captureEmbeddingRequest(t *testing.T, body map[string]any, seen *map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		(*seen)["authorization"] = r.Header.Get("Authorization")
		(*seen)["path"] = r.URL.Path
		(*seen)["body"] = string(payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGenerateEmbeddingOllamaSuccess(t *testing.T) {
	want := dimensionVector(1024)
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": want})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Photovoltaik Speicher")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1024 {
		t.Fatalf("expected 1024 dimensions, got %d", len(got))
	}
	if got[1] != want[1] {
		t.Fatalf("expected real embedding values, got %v", got[1])
	}
}

func TestGenerateEmbeddingSanitizesPIIBeforeDispatchOllama(t *testing.T) {
	seen := map[string]string{}
	srv := captureEmbeddingRequest(t, map[string]any{"embedding": dimensionVector(1024)}, &seen)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})

	_, err := gw.GenerateEmbedding(context.Background(), "Kunde zahlt via IBAN DE89370400440532013000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(seen["body"], "DE89370400440532013000") {
		t.Fatalf("raw IBAN leaked to embedding provider: %s", seen["body"])
	}
	if !strings.Contains(seen["body"], "[REDACTED_IBAN]") {
		t.Fatalf("expected sanitized prompt, got: %s", seen["body"])
	}
}

func TestGenerateEmbeddingSanitizesPIIBeforeDispatchOpenAICompatible(t *testing.T) {
	seen := map[string]string{}
	srv := captureEmbeddingRequest(t, map[string]any{
		"data": []map[string]any{{"embedding": dimensionVector(1024)}},
	}, &seen)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderNebius,
		AIBaseURL:       srv.URL,
		EmbeddingModel:  "Qwen/Qwen3-Embedding-8B",
		APIKey:          "test-key",
	})

	_, err := gw.GenerateEmbedding(context.Background(), "Kunde zahlt via IBAN DE89370400440532013000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(seen["body"], "DE89370400440532013000") {
		t.Fatalf("raw IBAN leaked to embedding provider: %s", seen["body"])
	}
	if !strings.Contains(seen["body"], "[REDACTED_IBAN]") {
		t.Fatalf("expected sanitized prompt, got: %s", seen["body"])
	}
}

func TestGenerateEmbeddingWrongDimensionsIsTypedError(t *testing.T) {
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": dimensionVector(512)})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "tiny-model",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatalf("expected dimension mismatch error, got vector of len %d", len(got))
	}
	if !errors.Is(err, ai.ErrEmbeddingDimensionMismatch) {
		t.Fatalf("expected ErrEmbeddingDimensionMismatch, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no fabricated vector on mismatch, got %v", got)
	}
	for _, want := range []string{"tiny-model", "512", "1024"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error %q to name %q", err.Error(), want)
		}
	}
}

func TestGenerateEmbeddingUpstreamStatusReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatal("expected honest error on upstream failure")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no fabricated vector, got %v", got)
	}
}

func TestGenerateEmbeddingUnreachableReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable := srv.URL
	srv.Close()
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   unreachable,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatal("expected honest error when backend unreachable")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no fabricated vector, got %v", got)
	}
}

func TestGenerateEmbeddingOpenAICompatibleSuccess(t *testing.T) {
	want := dimensionVector(1024)
	seen := map[string]string{}
	srv := captureEmbeddingRequest(t, map[string]any{
		"data": []map[string]any{{"embedding": want}},
	}, &seen)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderNebius,
		AIBaseURL:       srv.URL,
		EmbeddingModel:  "Qwen/Qwen3-Embedding-8B",
		APIKey:          "test-key",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Photovoltaik Speicher")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1024 {
		t.Fatalf("expected 1024 dimensions, got %d", len(got))
	}
	if seen["path"] != "/embeddings" {
		t.Fatalf("expected OpenAI-compatible /embeddings endpoint, got %q", seen["path"])
	}
	if seen["authorization"] != "Bearer test-key" {
		t.Fatalf("expected Bearer auth header, got %q", seen["authorization"])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(seen["body"]), &payload); err != nil {
		t.Fatalf("invalid request body: %v", err)
	}
	if payload["model"] != "Qwen/Qwen3-Embedding-8B" {
		t.Fatalf("expected configured model in request, got %v", payload["model"])
	}
}

func TestGenerateEmbeddingOpenAICompatibleStatusErrorReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderMistral,
		AIBaseURL:       srv.URL,
		EmbeddingModel:  "mistral-embed",
		APIKey:          "test-key",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatal("expected honest error on upstream failure")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no fabricated vector, got %v", got)
	}
}

func TestGenerateEmbeddingOpenAICompatibleUnreachableReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable := srv.URL
	srv.Close()
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		AIBaseURL:       unreachable,
		EmbeddingModel:  "text-embedding-3-small",
	})

	_, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatal("expected honest error when backend unreachable")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestGenerateEmbeddingProvidersWithoutEmbeddingsAPIAreHonest(t *testing.T) {
	cases := []struct {
		provider ai.Provider
	}{
		{ai.ProviderAnthropic},
		{ai.ProviderDeepSeek},
		{ai.Provider("totally-unknown")},
	}
	for _, tc := range cases {
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: tc.provider,
			AIBaseURL:       "http://localhost:1", // must never be contacted
			EmbeddingModel:  "some-model",
		})

		got, err := gw.GenerateEmbedding(context.Background(), "Text")
		if err == nil {
			t.Fatalf("provider %s: expected honest error, got fabricated vector %v", tc.provider, got)
		}
		if !errors.Is(err, ai.ErrEmbeddingsUnsupported) {
			t.Fatalf("provider %s: expected ErrEmbeddingsUnsupported, got: %v", tc.provider, err)
		}
		if !strings.Contains(err.Error(), string(tc.provider)) {
			t.Fatalf("provider %s: error must name the provider, got %q", tc.provider, err.Error())
		}
	}
}

func TestGenerateEmbeddingOpenAICompatibleWithoutModelIsHonestConfigError(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": dimensionVector(1024)}}})
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		AIBaseURL:       srv.URL,
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err == nil {
		t.Fatalf("expected honest config error, got vector of len %d", len(got))
	}
	if !errors.Is(err, ai.ErrEmbeddingModelNotConfigured) {
		t.Fatalf("expected ErrEmbeddingModelNotConfigured, got: %v", err)
	}
	if called {
		t.Fatal("expected no upstream call without a configured embedding model")
	}
	if !strings.Contains(err.Error(), "AI_EMBEDDING_MODEL") {
		t.Fatalf("error must name the missing variable, got %q", err.Error())
	}
}

func TestValidateEmbeddingModelDetectsMismatch(t *testing.T) {
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": dimensionVector(1536)})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "wrong-model",
	})

	err := gw.ValidateEmbeddingModel(context.Background())
	if !errors.Is(err, ai.ErrEmbeddingDimensionMismatch) {
		t.Fatalf("expected ErrEmbeddingDimensionMismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1536") || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("mismatch must name got/expected dims, got %q", err.Error())
	}
}

func TestValidateEmbeddingModelUnreachableIsDistinguishable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable := srv.URL
	srv.Close()
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   unreachable,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})

	err := gw.ValidateEmbeddingModel(context.Background())
	if err == nil {
		t.Fatal("expected error when backend unreachable")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if errors.Is(err, ai.ErrEmbeddingDimensionMismatch) {
		t.Fatalf("unreachable must not be reported as dimension mismatch: %v", err)
	}
}

func TestValidateEmbeddingModelUnsupportedProviderIsHonest(t *testing.T) {
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderAnthropic,
		EmbeddingModel:  "whatever",
	})

	err := gw.ValidateEmbeddingModel(context.Background())
	if !errors.Is(err, ai.ErrEmbeddingsUnsupported) {
		t.Fatalf("expected ErrEmbeddingsUnsupported, got: %v", err)
	}
}

func TestNewGatewayResolvesEmbeddingModelPrecedence(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "ai-embedding-model")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "ollama-embedding-model")
	gw := ai.NewGateway(ai.GatewayConfig{DefaultProvider: ai.ProviderOllama})
	if got := gw.GetConfig().EmbeddingModel; got != "ai-embedding-model" {
		t.Fatalf("expected AI_EMBEDDING_MODEL to win, got %q", got)
	}
}

func TestNewGatewayFallsBackToOllamaEmbeddingModelEnv(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "legacy-env-model")
	gw := ai.NewGateway(ai.GatewayConfig{DefaultProvider: ai.ProviderOllama})
	if got := gw.GetConfig().EmbeddingModel; got != "legacy-env-model" {
		t.Fatalf("expected OLLAMA_EMBEDDING_MODEL fallback, got %q", got)
	}
}

func TestNewGatewayOllamaDefaultEmbeddingModel(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "")
	gw := ai.NewGateway(ai.GatewayConfig{DefaultProvider: ai.ProviderOllama})
	if got := gw.GetConfig().EmbeddingModel; got != "qwen3-embedding:0.6b" {
		t.Fatalf("expected default qwen3-embedding:0.6b, got %q", got)
	}
}

func TestNewGatewayOpenAICompatibleHasNoDefaultEmbeddingModel(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "")
	gw := ai.NewGateway(ai.GatewayConfig{DefaultProvider: ai.ProviderOpenAI})
	if got := gw.GetConfig().EmbeddingModel; got != "" {
		t.Fatalf("expected no guessed model for openai-compatible providers, got %q", got)
	}
}
