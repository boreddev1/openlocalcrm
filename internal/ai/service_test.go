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

	resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
		Messages: []ai.ChatMessage{
			{Role: "user", Content: "Was ist der aktuelle Status von Dr. Weber?"},
		},
		Context: "Deals Pipeline: 5 open deals",
	})
	if err != nil {
		t.Fatalf("expected successful chat response, got: %v", err)
	}
	if resp.Reply == "" {
		t.Fatalf("expected non-empty copilot response")
	}
}
