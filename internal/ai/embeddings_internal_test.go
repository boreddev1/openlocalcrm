package ai

import "testing"

func TestEmbeddingsBaseURLPerProviderDefaults(t *testing.T) {
	cases := []struct {
		provider Provider
		want     string
	}{
		{ProviderOpenAI, "https://api.openai.com/v1"},
		{ProviderMistral, "https://api.mistral.ai/v1"},
		{ProviderNebius, "https://api.studio.nebius.ai/v1"},
	}
	for _, tc := range cases {
		g := NewGateway(GatewayConfig{DefaultProvider: tc.provider})
		if got := g.embeddingsBaseURL(); got != tc.want {
			t.Fatalf("provider %s: expected base URL %q, got %q", tc.provider, tc.want, got)
		}
	}
}

func TestEmbeddingsBaseURLAIRelativeOverride(t *testing.T) {
	g := NewGateway(GatewayConfig{
		DefaultProvider: ProviderNebius,
		AIBaseURL:       "http://localhost:9999/v1/",
	})
	if got := g.embeddingsBaseURL(); got != "http://localhost:9999/v1" {
		t.Fatalf("expected AI_BASE_URL override without trailing slash, got %q", got)
	}
}
