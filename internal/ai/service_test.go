package ai_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestPromptGuardPIIMasking(t *testing.T) {
	guard := ai.NewGuard()

	input := "Kunde mit IBAN DE89370400440532013000 und Kreditkarte 4111 2222 3333 4444, Steuer-ID 12345678901, USt-IdNr. DE987654321 sendet api_key='sk_live_1234567890123456'."
	sanitized, count := guard.SanitizeInputWithCount(input)

	if sanitized == input {
		t.Fatalf("expected PII redaction, got unchanged input")
	}

	if count != 5 {
		t.Fatalf("expected 5 redactions (IBAN, CC, TaxID, VatID, APIKey), got %d", count)
	}

	if contains(sanitized, "DE89370400440532013000") {
		t.Fatalf("IBAN was not redacted!")
	}

	if contains(sanitized, "4111 2222 3333 4444") {
		t.Fatalf("Credit card was not redacted!")
	}

	if contains(sanitized, "12345678901") {
		t.Fatalf("Steuer-ID was not redacted!")
	}

	if contains(sanitized, "DE987654321") {
		t.Fatalf("USt-IdNr was not redacted!")
	}

	if contains(sanitized, "sk_live_1234567890123456") {
		t.Fatalf("API key was not redacted!")
	}
}

func TestPromptGuardOutputValidation(t *testing.T) {
	guard := ai.NewGuard()

	// Script injection
	maliciousOutput := `Hier ist das Ergebnis: <script>alert("hacked")</script> und <script src="evil.js">`
	cleaned, err := guard.ValidateOutput(maliciousOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(cleaned, "<script>") || contains(cleaned, `alert("hacked")`) {
		t.Fatalf("script injection was not blocked!")
	}

	// System prompt leak
	leakOutput := `Ich handle nach System Instruction: <untrusted_user_context>admin</untrusted_user_context>`
	cleanedLeak, err := guard.ValidateOutput(leakOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(cleanedLeak, "System Instruction:") || contains(cleanedLeak, "<untrusted_user_context>") {
		t.Fatalf("system prompt leak was not redacted!")
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

	t.Run("customer creation and research intent creates company and returns action card", func(t *testing.T) {
		querier := demo.NewInMemoryQuerier()
		chatWithDB := ai.NewChatService(gw, obs, querier)

		prompt := "kannst du die Bäckerei passa als kunden anlegen udn die meta information für den Kunden rechevcheiren und anlegen?"
		resp, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "assistant", Content: "Hallo! Ich bin Ihr OpenLocalCRM KI-Vertriebs-Copilot."},
				{Role: "user", Content: prompt},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !contains(resp.Reply, "Bäckerei Passa") {
			t.Errorf("expected reply to mention Bäckerei Passa, got: %s", resp.Reply)
		}
		if contains(resp.Reply, "Hallo! Ich bin Ihr OpenLocalCRM Vertriebs-Copilot") {
			t.Errorf("greeting loop detected! Should have executed creation, got greeting: %s", resp.Reply)
		}
		if resp.ActionCard == nil || resp.ActionCard.Route != "/companies" {
			t.Errorf("expected ActionCard to /companies, got: %+v", resp.ActionCard)
		}

		// Verify company was created in querier
		companies, _ := querier.ListCompanies(ctx, db.ListCompaniesParams{Limit: 10, Offset: 0})
		found := false
		for _, c := range companies {
			if c.Name == "Bäckerei Passa" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Bäckerei Passa to be created in DB, but not found in %v", companies)
		}
	})

	t.Run("custom model mistral is preserved in response", func(t *testing.T) {
		mistralGw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaModel:     "mistral",
		})
		mistralChat := ai.NewChatService(mistralGw, obs)

		resp, err := mistralChat.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Hallo"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Model != "mistral" {
			t.Errorf("expected model mistral, got: %s", resp.Model)
		}
	})
}
