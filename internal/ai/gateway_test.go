package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func fakeOllamaServer(t *testing.T, status int, response string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "upstream failure", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": response})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeRawServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGatewayGenerateOllamaSuccess(t *testing.T) {
	srv := fakeOllamaServer(t, http.StatusOK, "Echte Modellantwort")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Echte Modellantwort" {
		t.Fatalf("expected real model response, got: %q", got)
	}
}

func TestGatewayGenerateOllamaStatusErrorReturnsHonestError(t *testing.T) {
	srv := fakeOllamaServer(t, http.StatusInternalServerError, "")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err == nil {
		t.Fatalf("expected honest error on provider failure, got fabricated reply: %q", got)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty reply on provider failure, got: %q", got)
	}
}

func TestGatewayGenerateOllamaUnreachableReturnsHonestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := srv.URL
	srv.Close()

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   unreachableURL,
		OllamaModel:     "test-model",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err == nil {
		t.Fatalf("expected honest error when provider is unreachable, got fabricated reply: %q", got)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestGatewayGenerateOpenAIStatusErrorReturnsHonestError(t *testing.T) {
	srv := fakeOllamaServer(t, http.StatusBadGateway, "")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "gpt-4o-mini",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err == nil {
		t.Fatalf("expected honest error on provider failure, got fabricated reply: %q", got)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestGatewayGenerateOpenAISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "OpenAI Antwort"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "gpt-4o-mini",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "OpenAI Antwort" {
		t.Fatalf("expected OpenAI response, got: %q", got)
	}
}

func TestGatewayGenerateInvalidBaseURLReturnsHonestError(t *testing.T) {
	const invalidURL = "http://bad url/"

	t.Run("ollama", func(t *testing.T) {
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   invalidURL,
			OllamaModel:     "test-model",
		})
		if _, err := gw.Generate(context.Background(), "Hallo", ""); !errors.Is(err, ai.ErrUpstreamUnavailable) {
			t.Fatalf("expected ErrUpstreamUnavailable for invalid base URL, got: %v", err)
		}
	})

	t.Run("openai", func(t *testing.T) {
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOpenAI,
			OllamaBaseURL:   invalidURL,
			OllamaModel:     "gpt-4o-mini",
		})
		if _, err := gw.Generate(context.Background(), "Hallo", ""); !errors.Is(err, ai.ErrUpstreamUnavailable) {
			t.Fatalf("expected ErrUpstreamUnavailable for invalid base URL, got: %v", err)
		}
	})
}

func TestGatewayGenerateOllamaNonJSONReturnsHonestError(t *testing.T) {
	srv := fakeRawServer(t, http.StatusOK, "this is not json")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err == nil {
		t.Fatalf("expected honest error on unparseable upstream body, got: %q", got)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestGatewayGenerateOpenAINonJSONReturnsHonestError(t *testing.T) {
	srv := fakeRawServer(t, http.StatusOK, "this is not json")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "gpt-4o-mini",
	})

	got, err := gw.Generate(context.Background(), "Hallo", "")
	if err == nil {
		t.Fatalf("expected honest error on unparseable upstream body, got: %q", got)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestTriageEmailUpstreamFailureReturnsHonestError(t *testing.T) {
	srv := fakeOllamaServer(t, http.StatusInternalServerError, "")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
	svc := ai.NewTriageService(gw)

	_, err := svc.TriageEmail(context.Background(), "kunde@solar.de", "Angebot", "Bitte um Angebot")
	if err == nil {
		t.Fatal("expected honest error on triage provider failure")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestTriageEmailUsesRealModelResponse(t *testing.T) {
	payload := `{"category":"SUPPORT","sentiment":"NEGATIVE","priority":"URGENT","summary":"Modell-Zusammenfassung","draft_reply":"Modell-Antwort"}`
	srv := fakeOllamaServer(t, http.StatusOK, payload)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
	svc := ai.NewTriageService(gw)

	res, err := svc.TriageEmail(context.Background(), "kunde@solar.de", "Angebot", "Bitte um Angebot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Category != "SUPPORT" {
		t.Fatalf("expected model category SUPPORT, got %q", res.Category)
	}
	if res.Summary != "Modell-Zusammenfassung" {
		t.Fatalf("expected model summary, got %q", res.Summary)
	}
	if !strings.Contains(res.DraftReply, "Modell-Antwort") {
		t.Fatalf("expected model draft reply, got %q", res.DraftReply)
	}
}
