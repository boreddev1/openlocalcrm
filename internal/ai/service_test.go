package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	payload := `{"category":"ANFRAGE","sentiment":"POSITIVE","priority":"HIGH","summary":"Kunde möchte ein Angebot.","draft_reply":"Vielen Dank für Ihre Anfrage."}`
	srv := fakeOllamaServer(t, http.StatusOK, payload)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
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

	if res.Summary != "Kunde möchte ein Angebot." {
		t.Errorf("expected model summary, got %s", res.Summary)
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

	t.Run("returns real model reply without simulation flag", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Antwort vom echten Modell")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		chatSvc := ai.NewChatService(gw, obs, nil)

		resp, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "hi"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Reply != "Antwort vom echten Modell" {
			t.Fatalf("expected real model reply, got: %s", resp.Reply)
		}
		if resp.Simulated {
			t.Fatalf("simulated flag must never be set")
		}
		if contains(resp.Reply, "106.700 €") {
			t.Fatalf("reply should never contain hardcoded fake pipeline volume")
		}
	})

	t.Run("provider failure returns honest upstream error", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusInternalServerError, "")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		chatSvc := ai.NewChatService(gw, obs, nil)

		_, err := chatSvc.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Fasse die Pipeline zusammen"},
			},
		})
		if err == nil {
			t.Fatal("expected honest error when provider fails, got canned reply")
		}
		if !errors.Is(err, ai.ErrUpstreamUnavailable) {
			t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
		}
	})

	t.Run("pipeline summary returns action card", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Hier ist Ihre aktuelle Pipeline.")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		chatSvc := ai.NewChatService(gw, obs, nil)

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
		srv := fakeOllamaServer(t, http.StatusOK, "Hier sind Ihre Automationen.")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		chatSvc := ai.NewChatService(gw, obs, nil)

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

	t.Run("customer creation requires confirmation before creating company", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Antwort vom echten Modell")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		querier := demo.NewInMemoryQuerier()
		chatWithDB := ai.NewChatService(gw, obs, nil, querier)

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
		if resp.ActionCard == nil || resp.ActionCard.Badge != "Bestätigung erforderlich" {
			t.Errorf("expected confirmation ActionCard, got: %+v", resp.ActionCard)
		}

		// Verify company is NOT created without confirmation (Finding H3)
		companies, _ := querier.ListCompanies(ctx, db.ListCompaniesParams{Limit: 10, Offset: 0})
		for _, c := range companies {
			if c.Name == "Bäckerei Passa" {
				t.Fatalf("company Bäckerei Passa should not be created without confirmation")
			}
		}

		// Now confirm action
		respConfirmed, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "assistant", Content: "Hallo! Ich bin Ihr OpenLocalCRM KI-Vertriebs-Copilot."},
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: resp.Reply},
				{Role: "user", Content: "Ja, bitte ausführen und bestätigen."},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error on confirmation: %v", err)
		}

		if respConfirmed.ActionCard == nil || respConfirmed.ActionCard.Route != "/companies" {
			t.Errorf("expected ActionCard to /companies after confirmation, got: %+v", respConfirmed.ActionCard)
		}

		// With no research service available the reply must be honest, not fabricated.
		if contains(respConfirmed.Reply, "Recherchierte Unternehmensdaten") ||
			contains(respConfirmed.Reply, "PV-Potenzial") ||
			contains(respConfirmed.Reply, "Gewerbe / B2B") {
			t.Errorf("must not fabricate research data, got: %s", respConfirmed.Reply)
		}
		if !contains(respConfirmed.Reply, "keine Recherchedaten") {
			t.Errorf("expected honest 'keine Recherchedaten' notice, got: %s", respConfirmed.Reply)
		}

		// Verify company was created in querier after confirmation
		companiesAfter, _ := querier.ListCompanies(ctx, db.ListCompaniesParams{Limit: 10, Offset: 0})
		found := false
		for _, c := range companiesAfter {
			if c.Name == "Bäckerei Passa" {
				found = true
				if contains(string(c.CustomFields), "pv_potential") ||
					contains(string(c.CustomFields), "Hohes Eigenverbrauchspotenzial") {
					t.Errorf("fabricated custom fields persisted: %s", c.CustomFields)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected Bäckerei Passa to be created in DB after confirmation, but not found in %v", companiesAfter)
		}
	})

	t.Run("research intent uses injected researcher data", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Antwort")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		querier := demo.NewInMemoryQuerier()
		researcher := fakeResearcher{result: ai.CompanyResearchResult{
			Domain:           "baeckerei-passa.de",
			Summary:          "Echte Recherche-Zusammenfassung",
			IndustryKeywords: []string{"Handwerk"},
		}}
		chatWithDB := ai.NewChatService(gw, obs, researcher, querier)

		prompt := "kannst du die Bäckerei passa als kunden anlegen und die meta information recherchieren?"
		resp, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		respConfirmed, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: resp.Reply},
				{Role: "user", Content: "Ja, bitte ausführen und bestätigen."},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error on confirmation: %v", err)
		}
		if !contains(respConfirmed.Reply, "Echte Recherche-Zusammenfassung") {
			t.Errorf("expected injected research summary in reply, got: %s", respConfirmed.Reply)
		}
		if contains(respConfirmed.Reply, "Recherchierte Unternehmensdaten") {
			t.Errorf("must not fabricate research data, got: %s", respConfirmed.Reply)
		}
	})

	t.Run("research failure yields honest keine Recherchedaten reply", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Antwort")
		gw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "gemma2:12b",
		})
		obs := ai.NewObservabilityService()
		querier := demo.NewInMemoryQuerier()
		researcher := fakeResearcher{err: ai.ErrUpstreamUnavailable}
		chatWithDB := ai.NewChatService(gw, obs, researcher, querier)

		prompt := "kannst du die Bäckerei passa als kunden anlegen und die meta information recherchieren?"
		resp, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		respConfirmed, err := chatWithDB.Chat(ctx, ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
				{Role: "assistant", Content: resp.Reply},
				{Role: "user", Content: "Ja, bitte ausführen und bestätigen."},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error on confirmation: %v", err)
		}
		if !contains(respConfirmed.Reply, "keine Recherchedaten") {
			t.Errorf("expected honest 'keine Recherchedaten' notice, got: %s", respConfirmed.Reply)
		}
	})

	t.Run("custom model mistral is preserved in response", func(t *testing.T) {
		srv := fakeOllamaServer(t, http.StatusOK, "Antwort vom echten Modell")
		mistralGw := ai.NewGateway(ai.GatewayConfig{
			DefaultProvider: ai.ProviderOllama,
			OllamaBaseURL:   srv.URL,
			OllamaModel:     "mistral",
		})
		obs := ai.NewObservabilityService()
		mistralChat := ai.NewChatService(mistralGw, obs, nil)

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

type fakeResearcher struct {
	result ai.CompanyResearchResult
	err    error
}

func (f fakeResearcher) ResearchCompany(ctx context.Context, domain string) (ai.CompanyResearchResult, error) {
	return f.result, f.err
}

func TestResearchCompanyNoUsableSourceDoesNotCallGateway(t *testing.T) {
	ctx := context.Background()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": `{"summary":"Erfundene Zusammenfassung"}`})
	}))
	t.Cleanup(srv.Close)
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
	svc := ai.NewResearchService(gw, ai.NewObservabilityService())

	res, err := svc.ResearchCompany(ctx, "example.invalid")
	if err == nil {
		t.Fatalf("expected honest error when no usable scraped source, got result: %+v", res)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("gateway.Generate must not be called without usable source, got %d calls", calls)
	}
	if res.Summary != "" {
		t.Fatalf("must not fabricate research summary, got: %q", res.Summary)
	}
	if len(res.IndustryKeywords) != 0 {
		t.Fatalf("must not fabricate industry keywords, got: %v", res.IndustryKeywords)
	}
}

func TestTriageEmailMalformedJSONReturnsUpstreamError(t *testing.T) {
	ctx := context.Background()
	srv := fakeOllamaServer(t, http.StatusOK, "das ist kein gültiges JSON")
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
	svc := ai.NewTriageService(gw)

	_, err := svc.TriageEmail(ctx, "kunde@solar.de", "Angebot", "Bitte um Angebot")
	if err == nil {
		t.Fatal("expected honest error on malformed upstream triage output")
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}
