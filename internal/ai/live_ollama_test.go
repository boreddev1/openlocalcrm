package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func TestLiveOllamaIntegrationWithPromptGuard(t *testing.T) {
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   "http://localhost:11434",
		OllamaModel:     "gemma4:12b",
	})
	triageSvc := ai.NewTriageService(gw)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Prompt with sensitive PII (IBAN & Secret Key)
	sender := "kunde@solardach-gmbh.de"
	subject := "Anfrage PV-Anlage & Speicher (IBAN DE89370400440532013000)"
	body := "Hallo OpenLocalCRM Team, wir möchten für unser Betriebsgebäude eine 25 kWp Solaranlage mit 15 kWh Speicher anfragen. Unsere interne Vorgangsnummer ist secret_token='sk_live_9988776655443322'. Bitte senden Sie uns zeitnah ein unverbindliches Angebot."

	res, err := triageSvc.TriageEmail(ctx, sender, subject, body)
	if err != nil {
		t.Fatalf("Live Ollama inference failed: %v", err)
	}

	t.Logf("=== LIVE OLLAMA (gemma4:12b) RESPONSE ===")
	t.Logf("Category:    %s", res.Category)
	t.Logf("Priority:    %s", res.Priority)
	t.Logf("Sentiment:   %s", res.Sentiment)
	t.Logf("Summary:     %s", res.Summary)
	t.Logf("Draft Reply: %s", res.DraftReply)

	if res.Category == "" {
		t.Errorf("Expected non-empty category")
	}
	if res.DraftReply == "" {
		t.Errorf("Expected non-empty draft reply")
	}
}
