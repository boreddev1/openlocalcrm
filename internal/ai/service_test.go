package ai_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func TestPromptGuardPIIMasking(t *testing.T) {
	guard := ai.NewGuard()

	input := "Kunde mit IBAN DE89370400440532013000 und Kreditkarte 4111 2222 3333 4444 sendet api_key='sk_live_1234567890123456'."
	sanitized := guard.SanitizeInput(input)

	if sanitized == input {
		t.Fatalf("expected PII redaction, got unchanged input")
	}

	if contains(sanitized, "DE89370400440532013000") {
		t.Fatalf("IBAN was not redacted!")
	}

	if contains(sanitized, "4111 2222 3333 4444") {
		t.Fatalf("Credit card was not redacted!")
	}

	if contains(sanitized, "sk_live_1234567890123456") {
		t.Fatalf("API key was not redacted!")
	}
}

func TestGemma12BEmailTriage(t *testing.T) {
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaModel:     "gemma2:12b",
	})
	triageSvc := ai.NewTriageService(gw)

	res, err := triageSvc.TriageEmail(
		context.Background(),
		"kunde@solardach-gmbh.de",
		"Anfrage PV-Anlage 20kWp",
		"Hallo, wir haben Interesse an einer 20 kWp PV-Anlage mit Speicher. Bitte Angebot zukommen lassen.",
	)

	if err != nil {
		t.Fatalf("expected successful triage, got: %v", err)
	}

	if res.Category != "ANFRAGE" {
		t.Errorf("expected category ANFRAGE, got %s", res.Category)
	}

	if res.DraftReply == "" {
		t.Errorf("expected generated draft reply")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestObservabilityService(t *testing.T) {
	ctx := context.Background()
	obs := ai.NewObservabilityService()

	obs.Record(ctx, ai.AIAuditLog{
		ID:                 "log-1",
		InteractionType:    "TRIAGE",
		ModelName:          "gemma2:12b",
		Provider:           "Ollama",
		LatencyMs:          120,
		PIIFilterTriggered: true,
		PIIRedactionsCount: 1,
	})

	logs := obs.GetRecentLogs(10)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	stats := obs.GetStats()
	if stats.TotalInteractions != 1 {
		t.Fatalf("expected 1 interaction, got %d", stats.TotalInteractions)
	}
	if stats.TotalPIIBlocked != 1 {
		t.Fatalf("expected 1 PII blocked, got %d", stats.TotalPIIBlocked)
	}
}

func TestCopilotChatService(t *testing.T) {
	ctx := context.Background()
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaModel:     "gemma2:12b",
	})
	obs := ai.NewObservabilityService()
	chatSvc := ai.NewChatService(gw, obs)

	t.Run("greeting hi does not return static deal volume", func(t *testing.T) {
		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "hi"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(resp.Reply, "Vertriebs-Copilot") {
			t.Fatalf("expected greeting reply, got: %s", resp.Reply)
		}
		if contains(resp.Reply, "106.700 €") {
			t.Fatalf("greeting should never contain hardcoded fake pipeline volume")
		}
	})

	t.Run("gibberish dd does not return static deal volume", func(t *testing.T) {
		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "dd"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(resp.Reply, "nicht genau verstanden") {
			t.Fatalf("expected helpful fallback for unknown input, got: %s", resp.Reply)
		}
		if contains(resp.Reply, "106.700 €") {
			t.Fatalf("dd input should never contain hardcoded fake pipeline volume")
		}
	})

	t.Run("pipeline summary returns action card", func(t *testing.T) {
		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Fasse die Pipeline zusammen"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ActionCard == nil || resp.ActionCard.Route != "/deals" {
			t.Fatalf("expected action card with /deals, got: %+v", resp.ActionCard)
		}
	})

	t.Run("workflow request returns automations action card", func(t *testing.T) {
		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Workflow für Neukunden anlegen"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ActionCard == nil || resp.ActionCard.Route != "/automations" {
			t.Fatalf("expected action card with /automations, got: %+v", resp.ActionCard)
		}
	})

	t.Run("legal BGB 355 advice", func(t *testing.T) {
		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Wie ist die Widerrufsfrist nach § 355 BGB?"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(resp.Reply, "14 Tage") {
			t.Fatalf("expected 14 days legal advice, got: %s", resp.Reply)
		}
	})
}
