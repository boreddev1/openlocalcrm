package ai_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

// Live test against a real OpenAI-compatible endpoint (homelab). Gated: it only runs
// when AI_LIVE_TEST=1 and the env carries base URL, model and key. The API key is
// read exclusively from the environment (AI_API_KEY) and is NEVER a literal in this
// file. It asserts the served model identity, a real completion, real embeddings,
// and that the PII guard holds when a genuine model does the work.
//
// Adopted for issue #82. The PII subtest runs three iterations so the
// non-deterministic leak class cannot hide behind a single clean run.
func TestLiveOpenAICompatibleGateway(t *testing.T) {
	if os.Getenv("AI_LIVE_TEST") != "1" {
		t.Skip("set AI_LIVE_TEST=1 to run the live OpenAI-compatible integration test")
	}
	base := os.Getenv("AI_BASE_URL")
	model := os.Getenv("AI_MODEL")
	key := os.Getenv("AI_API_KEY")
	embed := os.Getenv("AI_EMBEDDING_MODEL")
	if base == "" || model == "" || key == "" {
		t.Skip("AI_BASE_URL, AI_MODEL and AI_API_KEY must all be set")
	}

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOpenAI,
		AIBaseURL:       base,
		OllamaBaseURL:   os.Getenv("AI_CHAT_BASE_VIA_OLLAMA_FIELD"),
		APIKey:          key,
		EmbeddingModel:  embed,
	})

	t.Run("serving model is what was configured", func(t *testing.T) {
		if got := gw.ServingModel(); !strings.Contains(got, strings.Split(model, "/")[0]) && got != model {
			t.Fatalf("ServingModel() = %q, want it to reflect %q", got, model)
		}
	})

	t.Run("real completion comes back", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		start := time.Now()
		answer, err := gw.Generate(ctx, "Antworte mit genau einem Wort: Bereit", "Du antwortest extrem knapp.")
		latency := time.Since(start)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if strings.TrimSpace(answer) == "" {
			t.Fatal("Generate() returned an empty answer from a real endpoint")
		}
		if strings.Contains(answer, "DEMO") {
			t.Fatalf("real endpoint returned the demo marker, stub leaked: %q", answer)
		}
		t.Logf("completion latency: %s", latency)
		t.Logf("antwort: %q", firstRunes(answer, 80))
	})

	t.Run("real embeddings", func(t *testing.T) {
		if embed == "" {
			t.Skip("AI_EMBEDDING_MODEL not set")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		start := time.Now()
		vec, err := gw.GenerateEmbedding(ctx, "Photovoltaik Anlage Dach")
		latency := time.Since(start)
		if err != nil {
			t.Fatalf("GenerateEmbedding() error = %v", err)
		}
		if len(vec) < 16 {
			t.Fatalf("GenerateEmbedding() returned %d dims, expected a real vector", len(vec))
		}
		t.Logf("embedding latency: %s (%d dims)", latency, len(vec))
	})

	// The guard must hold against a genuine model. Three iterations: on every run
	// either the leak never reaches the released output, or the guard refuses to
	// release it at all (ErrPIILeakDetected). A leak that passes through is a FAIL.
	// A non-guard error (transport/parse) is a real failure, not a pass.
	t.Run("PII guard holds on a real model", func(t *testing.T) {
		const forbidden = "meier@gmbh.de"
		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			start := time.Now()
			svc := ai.NewTriageService(gw)
			out, err := svc.TriageEmail(ctx,
				"m.meier@gmbh.de",
				"Anfrage Angebot",
				"Sehr geehrte Damen, meier@gmbh.de, Telefon 0171 2345678, bitte das Angebot fuer die Anlage senden.",
			)
			latency := time.Since(start)

			if err != nil {
				if errors.Is(err, ai.ErrPIILeakDetected) {
					t.Logf("run %d: guard rejected a leaked e-mail (%s) — no leak surfaced", i+1, latency)
					continue
				}
				t.Fatalf("run %d: TriageEmail() failed for a non-PII reason: %v", i+1, err)
			}

			blob := out.Summary + " " + out.Category + " " + out.DraftReply
			for _, leak := range []string{forbidden, "0171 2345678", "2345678"} {
				if strings.Contains(blob, leak) {
					t.Fatalf("run %d: PII leaked to the triage output: %q (%s)", i+1, leak, latency)
				}
			}
			t.Logf("run %d: clean output, no PII leaked (%s)", i+1, latency)
		}
	})
}

func firstRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
