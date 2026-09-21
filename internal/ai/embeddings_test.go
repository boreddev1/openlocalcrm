package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestGenerateEmbeddingOllamaSuccess(t *testing.T) {
	want := dimensionVector(768)
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": want})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        srv.URL,
		OllamaModel:          "test-model",
		OllamaEmbeddingModel: "nomic-embed-text",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Photovoltaik Speicher")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 768 {
		t.Fatalf("expected 768 dimensions, got %d", len(got))
	}
	if got[1] != want[1] {
		t.Fatalf("expected real embedding values, got %v", got[1])
	}
}

func TestGenerateEmbeddingWrongDimensionsIsTypedError(t *testing.T) {
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": dimensionVector(10)})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        srv.URL,
		OllamaEmbeddingModel: "tiny-model",
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
}

func TestGenerateEmbeddingUpstreamStatusReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        srv.URL,
		OllamaEmbeddingModel: "nomic-embed-text",
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
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        unreachable,
		OllamaEmbeddingModel: "nomic-embed-text",
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

func TestGenerateEmbeddingOpenAISuccess(t *testing.T) {
	want := dimensionVector(768)
	srv := fakeEmbeddingServer(t, map[string]any{
		"data": []map[string]any{{"embedding": want}},
	})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOpenAI,
		OllamaBaseURL:        srv.URL,
		OllamaEmbeddingModel: "text-embedding-3-small",
		APIKey:               "test-key",
	})

	got, err := gw.GenerateEmbedding(context.Background(), "Text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 768 {
		t.Fatalf("expected 768 dimensions, got %d", len(got))
	}
}

func TestValidateEmbeddingModelDetectsMismatch(t *testing.T) {
	srv := fakeEmbeddingServer(t, map[string]any{"embedding": dimensionVector(4)})
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        srv.URL,
		OllamaEmbeddingModel: "tiny-model",
	})

	err := gw.ValidateEmbeddingModel(context.Background())
	if !errors.Is(err, ai.ErrEmbeddingDimensionMismatch) {
		t.Fatalf("expected ErrEmbeddingDimensionMismatch, got: %v", err)
	}
}

func TestValidateEmbeddingModelUnreachableIsDistinguishable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable := srv.URL
	srv.Close()
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        unreachable,
		OllamaEmbeddingModel: "nomic-embed-text",
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

func TestNewGatewayResolvesEmbeddingModelFromEnv(t *testing.T) {
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "custom-embed-model")
	gw := ai.NewGateway(ai.GatewayConfig{})
	if got := gw.GetConfig().OllamaEmbeddingModel; got != "custom-embed-model" {
		t.Fatalf("expected env-resolved embedding model, got %q", got)
	}
}

func TestNewGatewayDefaultsEmbeddingModel(t *testing.T) {
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "")
	gw := ai.NewGateway(ai.GatewayConfig{})
	if got := gw.GetConfig().OllamaEmbeddingModel; got != "nomic-embed-text" {
		t.Fatalf("expected default nomic-embed-text, got %q", got)
	}
}
