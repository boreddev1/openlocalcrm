package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestProbeAIConnection_OllamaSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma2:12b"}]}`))
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer ts.Close()

	res := ProbeAIConnection(context.Background(), AIProbeRequest{
		Provider: "openai",
		BaseURL:  ts.URL,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})

	if !res.Success {
		t.Fatalf("expected successful probe, got error: %s", res.Message)
	}
}
