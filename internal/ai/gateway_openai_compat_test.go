package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

// captureChatServer records whether it was hit and the last request it saw, and
// answers with an OpenAI-compatible chat completion so the gateway path completes.
type captureChatServer struct {
	srv  *httptest.Server
	hits int32
	auth string
	path string
	body string
}

func newCaptureChatServer(t *testing.T) *captureChatServer {
	t.Helper()
	c := &captureChatServer{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&c.hits, 1)
		payload, _ := io.ReadAll(r.Body)
		c.auth = r.Header.Get("Authorization")
		c.path = r.URL.Path
		c.body = string(payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Bereit"}},
			},
		})
	}))
	t.Cleanup(c.srv.Close)
	return c
}

// Issue #82: a configured OpenAI-compatible base URL must be honored for chat, not
// only for embeddings. Resolution order: AIBaseURL, then a non-Ollama OllamaBaseURL,
// then the api.openai.com default.
func TestChatHonorsConfiguredAIBaseURL(t *testing.T) {
	aiSrv := newCaptureChatServer(t)
	staleSrv := newCaptureChatServer(t) // would only be used by the buggy OllamaBaseURL path

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderOpenAI,
		DisableEnvFallback: true,
		AIBaseURL:          aiSrv.srv.URL,
		OllamaBaseURL:      staleSrv.srv.URL,
		APIKey:             "sekret-key",
	})

	out, err := gw.Generate(context.Background(), "Antworte knapp", "Du antwortest kurz.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Bereit" {
		t.Fatalf("unexpected completion: %q", out)
	}
	if atomic.LoadInt32(&staleSrv.hits) != 0 {
		t.Fatalf("request went to the OllamaBaseURL instead of the configured AIBaseURL")
	}
	if atomic.LoadInt32(&aiSrv.hits) != 1 {
		t.Fatalf("expected exactly 1 request at the configured base, got %d", atomic.LoadInt32(&aiSrv.hits))
	}
	if aiSrv.path != "/chat/completions" {
		t.Fatalf("request arrived at wrong path: %q", aiSrv.path)
	}
	if aiSrv.auth != "Bearer sekret-key" {
		t.Fatalf("expected Authorization Bearer header, got: %q", aiSrv.auth)
	}
}

func TestChatHonorsNonOllamaOllamaBaseURLWhenNoAIBaseURL(t *testing.T) {
	srv := newCaptureChatServer(t)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderOpenAI,
		DisableEnvFallback: true,
		OllamaBaseURL:      srv.srv.URL, // non-11434 endpoint, no AIBaseURL set
		APIKey:             "k",
	})

	if _, err := gw.Generate(context.Background(), "hallo", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&srv.hits) != 1 {
		t.Fatalf("expected request at non-Ollama OllamaBaseURL, got %d hits", atomic.LoadInt32(&srv.hits))
	}
}

// Issue #82: ProviderMistral (and other declared OpenAI-compatible providers) must
// dispatch through the OpenAI-compatible path, not fall into the unknown-provider error.
func TestGenerateDispatchesMistralThroughOpenAICompatible(t *testing.T) {
	srv := newCaptureChatServer(t)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderMistral,
		DisableEnvFallback: true,
		AIBaseURL:          srv.srv.URL,
		APIKey:             "k",
	})

	out, err := gw.Generate(context.Background(), "hallo", "")
	if err != nil {
		t.Fatalf("mistral dispatch must not fail, got: %v", err)
	}
	if out != "Bereit" {
		t.Fatalf("unexpected completion: %q", out)
	}
	if atomic.LoadInt32(&srv.hits) != 1 {
		t.Fatalf("mistral request did not reach the OpenAI-compatible endpoint")
	}
}

func TestGenerateDispatchesNebiusThroughOpenAICompatible(t *testing.T) {
	srv := newCaptureChatServer(t)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderNebius,
		DisableEnvFallback: true,
		AIBaseURL:          srv.srv.URL,
		APIKey:             "k",
	})

	if _, err := gw.Generate(context.Background(), "hallo", ""); err != nil {
		t.Fatalf("nebius dispatch must not fail, got: %v", err)
	}
	if atomic.LoadInt32(&srv.hits) != 1 {
		t.Fatalf("nebius request did not reach the OpenAI-compatible endpoint")
	}
}

// Anthropic and Gemini must stay honest: not yet configured, never faked.
func TestGenerateKeepsAnthropicAndGeminiHonest(t *testing.T) {
	for _, p := range []ai.Provider{ai.ProviderAnthropic, ai.ProviderGemini} {
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider:    p,
			DisableEnvFallback: true,
			APIKey:             "k",
		})
		_, err := gw.Generate(context.Background(), "hallo", "")
		if err == nil {
			t.Fatalf("provider %q must return an honest not-configured error", p)
		}
	}
}

// The output-side guard must reject a mail leak at the transport layer for both the
// OpenAI-compatible and the Ollama call paths, and the rejection must remain
// identifiable as ErrPIILeakDetected (so callers never surface the raw PII).
func TestOpenAIPathRejectsEmailLeakInOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Antwort an leak@example.com gesendet."}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderOpenAI,
		DisableEnvFallback: true,
		AIBaseURL:          srv.URL,
		APIKey:             "k",
	})

	_, err := gw.Generate(context.Background(), "hallo", "")
	if err == nil {
		t.Fatal("expected the email leak to be rejected")
	}
	if !errors.Is(err, ai.ErrPIILeakDetected) {
		t.Fatalf("expected ErrPIILeakDetected, got: %v", err)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected the error to also wrap ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestOllamaPathRejectsEmailLeakInOutput(t *testing.T) {
	srv := fakeOllamaServer(t, http.StatusOK, `{"summary":"Bitte antworten an leak@example.com"}`)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderOllama,
		DisableEnvFallback: true,
		OllamaBaseURL:      srv.URL,
	})

	_, err := gw.Generate(context.Background(), "hallo", "")
	if err == nil {
		t.Fatal("expected the email leak to be rejected")
	}
	if !errors.Is(err, ai.ErrPIILeakDetected) {
		t.Fatalf("expected ErrPIILeakDetected, got: %v", err)
	}
}
