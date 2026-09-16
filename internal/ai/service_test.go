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
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && stringIndex(s, substr) >= 0))
}

func stringIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
