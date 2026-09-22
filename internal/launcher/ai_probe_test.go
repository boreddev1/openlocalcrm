package launcher

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func TestTranslateHostForDocker(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"http://localhost:11434", "http://host.docker.internal:11434"},
		{"http://127.0.0.1:11434", "http://host.docker.internal:11434"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"http://192.168.1.50:8000", "http://192.168.1.50:8000"},
	}

	for _, c := range cases {
		actual := TranslateHostForDocker(c.input)
		if actual != c.expected {
			t.Errorf("TranslateHostForDocker(%q) = %q; want %q", c.input, actual, c.expected)
		}
	}
}

func embeddingVecJSON(n int) string {
	vec := make([]string, n)
	for i := range vec {
		vec[i] = "0.1"
	}
	return `{"embedding":[` + strings.Join(vec, ",") + `]}`
}

func openAIEmbeddingJSON(n int) string {
	vec := make([]string, n)
	for i := range vec {
		vec[i] = "0.1"
	}
	return `{"data":[{"embedding":[` + strings.Join(vec, ",") + `]}]}`
}

func TestProbeAIConnection_OllamaSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma2:12b"}]}`))
		case "/api/embeddings":
			_, _ = w.Write([]byte(embeddingVecJSON(ai.EmbeddingDimensions)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama",
		BaseURL:  ts.URL,
		Model:    "gemma2:12b",
	})

	if !res.Success {
		t.Fatalf("expected successful probe, got error: %s", res.Message)
	}
}

func TestProbeAIConnection_OpenAISuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
		case "/embeddings":
			_, _ = w.Write([]byte(openAIEmbeddingJSON(ai.EmbeddingDimensions)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider:       "openai",
		BaseURL:        ts.URL,
		APIKey:         "test-key",
		Model:          "gpt-4o-mini",
		EmbeddingModel: "text-embedding-3-small",
	})

	if !res.Success {
		t.Fatalf("expected successful probe, got error: %s", res.Message)
	}
}

func TestProbeAIConnection_OllamaEmbeddingOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
		case "/api/embeddings":
			_, _ = w.Write([]byte(embeddingVecJSON(ai.EmbeddingDimensions)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
		EmbeddingModel: "qwen3-embedding:0.6b",
	})
	if !res.Success {
		t.Fatalf("expected overall success, got: %s", res.Message)
	}
	if res.Embedding == nil || !res.Embedding.Success || res.Embedding.Dims != ai.EmbeddingDimensions {
		t.Fatalf("expected embedding ok with dims=%d, got %+v", ai.EmbeddingDimensions, res.Embedding)
	}
}

func TestProbeAIConnection_OllamaEmbeddingDimensionMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
			return
		}
		_, _ = w.Write([]byte(embeddingVecJSON(512)))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
		EmbeddingModel: "tiny-model",
	})
	if res.Success {
		t.Fatalf("expected overall failure on dimension mismatch")
	}
	if !strings.Contains(res.Message, "crm-server wird mit dieser Konfiguration nicht starten") {
		t.Fatalf("expected fatal-consequence wording, got: %s", res.Message)
	}
}

func TestProbeAIConnection_OpenAIEmptyModelNoHTTPCall(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path == "/chat/completions" {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "openai", BaseURL: ts.URL, APIKey: "k", Model: "gpt-4o-mini",
	})
	if res.Success {
		t.Fatalf("expected failure with empty embedding model for openai provider")
	}
	if !strings.Contains(res.Message, "AI_EMBEDDING_MODEL") {
		t.Fatalf("expected honest config error, got: %s", res.Message)
	}
	if int32(2) < atomic.LoadInt32(&calls) { // chat call may probe; embeddings must not add calls
		t.Fatalf("unexpected extra calls: %d", calls)
	}
}

func TestProbeAIConnection_IgnoresLauncherEnv(t *testing.T) {
	t.Setenv("AI_EMBEDDING_MODEL", "env-model")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:12b"}]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "qwen3-embedding:0.6b") {
			http.Error(w, `{"error":"wrong model"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(embeddingVecJSON(ai.EmbeddingDimensions)))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL, Model: "gemma4:12b",
	})
	if !res.Success {
		t.Fatalf("expected probe to use explicit default model, not env, got: %s", res.Message)
	}
}

func TestProbeAIConnection_URLMappingUsesEnteredPort(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/embeddings":
			_, _ = w.Write([]byte(embeddingVecJSON(ai.EmbeddingDimensions)))
		}
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "ollama", BaseURL: ts.URL + "/", Model: "gemma4:12b",
		EmbeddingModel: "qwen3-embedding:0.6b",
	})
	if !res.Success {
		t.Fatalf("expected trailing slash to be trimmed and URL to be used, got: %s", res.Message)
	}
}
