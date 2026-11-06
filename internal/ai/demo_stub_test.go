package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

// demoProviderTrap is a provider server that fails the test when it is ever
// reached. In demo mode the gateway must answer without touching the network.
func demoProviderTrap(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("demo-mode gateway must not touch the network, got request to %s", r.URL.Path)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func demoGateway(t *testing.T) *ai.Gateway {
	t.Helper()
	srv := demoProviderTrap(t)
	return ai.NewGateway(ai.GatewayConfig{
		DemoMode:        true,
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "gemma4:12b",
	})
}

const demoChatSystem = "Du bist der OpenLocalCRM KI-Vertriebsassistent und Copilot."

const demoDraftSystem = "Du bist ein professioneller Kundenbetreuer im deutschen B2B- und Direktvertrieb. Verfasse eine höfliche, verbindliche E-Mail auf Deutsch. Format: Erste Zeile 'Betreff: <Betreffzeile>', gefolgt von einer Leerzeile und dem E-Mail-Text."

const demoNotesSystem = "Du bist ein präziser Vertriebs-Analyst. Antworte ausschließlich mit gültigem JSON."

const demoBillSystem = "Du bist ein präziser OCR-Parser für Energierechnungen. Antworte ausschließlich mit gültigem JSON."

const demoTemplateSystem = `Du bist ein E-Mail-Vorlagen-Generator für ein deutsches Vertriebs-CRM.
Antworte mit EXAKT einem JSON-Objekt und keinem anderen Text, ohne Markdown-Codeblock:
{"name":"<Anzeigename>","subject":"<Betreff mit {{contact_name}} Platzhaltern>","body":"<Mehrzeiliger Text>"}
Jedes Feld muss ein nicht-leerer String sein. Keine zusätzlichen Felder.`

// Requirement: the demo stub produces deterministic, marked content in demo
// mode — same input, same bytes, unmistakable DEMO marker, zero network.
func TestDemoGatewayChatStubIsDeterministicAndMarked(t *testing.T) {
	gw := demoGateway(t)

	first, err := gw.Generate(context.Background(), "USER: Wie ist der aktuelle Pipeline-Status?\n", demoChatSystem)
	if err != nil {
		t.Fatalf("demo chat stub failed: %v", err)
	}
	second, err := gw.Generate(context.Background(), "USER: Wie ist der aktuelle Pipeline-Status?\n", demoChatSystem)
	if err != nil {
		t.Fatalf("demo chat stub failed on second call: %v", err)
	}

	if !strings.Contains(first, ai.DemoStubMarker) {
		t.Fatalf("demo stub output must carry the unmistakable marker %q, got: %q", ai.DemoStubMarker, first)
	}
	if first != second {
		t.Fatalf("demo stub must return identical bytes for identical input, got differing outputs:\n%q\n%q", first, second)
	}

	// A second, independent gateway instance must produce the same bytes.
	gw2 := demoGateway(t)
	third, err := gw2.Generate(context.Background(), "USER: Wie ist der aktuelle Pipeline-Status?\n", demoChatSystem)
	if err != nil {
		t.Fatalf("demo chat stub failed on second gateway instance: %v", err)
	}
	if third != first {
		t.Fatalf("demo stub must be deterministic across gateway instances, got:\n%q\n%q", first, third)
	}
}

// The workflow HITL DRAFT_EMAIL step parses 'Betreff:' out of the generated
// text; the demo stub must satisfy that contract and still be marked.
func TestDemoGatewayDraftEmailStubMarksSubjectAndBody(t *testing.T) {
	gw := demoGateway(t)

	out, err := gw.Generate(context.Background(), "Empfänger: Dr. Michael Weber\nVorlage / Thema: Erstkontakt Photovoltaik\nErstelle einen passenden Erstkontakt-Entwurf.", demoDraftSystem)
	if err != nil {
		t.Fatalf("demo draft stub failed: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "Betreff:") {
		t.Fatalf("demo draft must start with a 'Betreff:' line so AIDrafter can parse it, got: %q", out)
	}
	if !strings.Contains(out, ai.DemoStubMarker) {
		t.Fatalf("demo draft body must carry the marker, got: %q", out)
	}
	if !strings.Contains(out, "keine Inferenz") {
		t.Fatalf("demo draft must state it ran no inference, got: %q", out)
	}
}

func TestDemoGatewayTriageStubProducesValidMarkedJSON(t *testing.T) {
	gw := demoGateway(t)
	svc := ai.NewTriageService(gw)

	res, err := svc.TriageEmail(context.Background(), "s.mueller@dachbau-nord.de", "Dringend: Rückmeldefrist", "Bitte um Rückmeldung.")
	if err != nil {
		t.Fatalf("demo triage stub failed: %v", err)
	}
	if res.Category == "" {
		t.Fatal("demo triage stub must fill category")
	}
	if !strings.Contains(res.DraftReply, ai.DemoStubMarker) {
		t.Fatalf("demo triage draft_reply must carry the marker, got: %q", res.DraftReply)
	}
	if !strings.Contains(res.Summary, ai.DemoStubMarker) {
		t.Fatalf("demo triage summary must carry the marker, got: %q", res.Summary)
	}
	// The stub must not echo attacker-controlled input into the generated draft.
	if strings.Contains(res.DraftReply, "Rückmeldefrist") || strings.Contains(res.DraftReply, "s.mueller") {
		t.Fatalf("demo triage stub must not echo input, got: %q", res.DraftReply)
	}
}

func TestDemoGatewayNotesSynthesisStubIsMarkedJSON(t *testing.T) {
	gw := demoGateway(t)

	out, err := gw.Generate(context.Background(), `Analysiere die folgenden Kundennotizen. Schema:
{"executive_summary": "string", "buying_intent": "string", "sentiment": "string", "key_objections": "string", "suggested_actions": [{"type": "CREATE_TODO|CREATE_DEAL", "label": "string"}]}
- [CALL] Max: Zählerstand notiert`, demoNotesSystem)
	if err != nil {
		t.Fatalf("demo notes synthesis stub failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("demo notes output must be valid JSON (ActivityLogDrawer re-marshals it): %v (raw %q)", err, out)
	}
	if s, _ := parsed["executive_summary"].(string); !strings.Contains(s, ai.DemoStubMarker) {
		t.Fatalf("demo executive_summary must carry the marker, got: %v", parsed["executive_summary"])
	}
	if bi, _ := parsed["buying_intent"].(string); bi == "" {
		t.Fatal("demo buying_intent must be filled — the UI renders 'Kaufbereitschaft: <value>'")
	}
	actions, ok := parsed["suggested_actions"].([]any)
	if !ok || len(actions) == 0 {
		t.Fatalf("demo suggested_actions must be a non-empty array, got: %v", parsed["suggested_actions"])
	}
}

func TestDemoGatewayParseBillStubIsMarkedJSONWithCalculatorFields(t *testing.T) {
	gw := demoGateway(t)

	out, err := gw.Generate(context.Background(), `Extrahiere aus folgendem Energierechnungs-Text die Daten für einen PV-Angebotsrechner im JSON-Format:
Text: demo
Erwartetes JSON-Format:
{"customer_name": "Name", "yearly_consumption": 6500, "current_electricity_price": 0.385, "meter_number": "1EMH...", "roof_area_sqm": 75, "roof_orientation": "Süd", "recommended_kwp": 14.5, "recommended_storage_kwh": 12.0}`, demoBillSystem)
	if err != nil {
		t.Fatalf("demo parse-bill stub failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("demo parse-bill output must be valid JSON (handler re-marshals it): %v (raw %q)", err, out)
	}
	for _, key := range []string{"customer_name", "yearly_consumption", "current_electricity_price", "meter_number", "roof_area_sqm", "roof_orientation", "recommended_kwp", "recommended_storage_kwh"} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("demo parse-bill JSON must contain %q, got: %v", key, parsed)
		}
	}
	if name, _ := parsed["customer_name"].(string); !strings.Contains(name, "DEMO") {
		t.Fatalf("demo customer_name must announce the stub (it lands in visible UI fields), got: %v", parsed["customer_name"])
	}
}

// The email-template generator keeps its honest failure in demo mode: the
// live-simulation spec asserts the UI reports "KI-Dienststelle konnte keine
// Vorlage liefern" instead of showing a canned template. The stub must never
// masquerade as a template generator.
func TestDemoGatewayEmailTemplateGenerationStaysAnHonestError(t *testing.T) {
	gw := demoGateway(t)

	out, err := gw.Generate(context.Background(), "Erstelle eine Nachfass-Mail für PV-Kunden.", demoTemplateSystem)
	if err == nil {
		t.Fatalf("demo mode must not fabricate email templates, got: %q", out)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable for template generation in demo mode, got: %v", err)
	}
}

func TestDemoGatewayUnknownInstructionIsHonestError(t *testing.T) {
	gw := demoGateway(t)

	out, err := gw.Generate(context.Background(), "Hallo", "Du bist ein antiker römischer Senator.")
	if err == nil {
		t.Fatalf("demo mode must refuse unknown intents instead of inventing output, got: %q", out)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
}

func TestDemoEmbeddingStubIsDeterministicAndSchemaCompliant(t *testing.T) {
	gw := demoGateway(t)

	vec, err := gw.GenerateEmbedding(context.Background(), "Photovoltaik & Speicher")
	if err != nil {
		t.Fatalf("demo embedding stub failed: %v", err)
	}
	if len(vec) != ai.EmbeddingDimensions {
		t.Fatalf("demo embedding must match the fixed pgvector width, got %d want %d", len(vec), ai.EmbeddingDimensions)
	}
	again, err := gw.GenerateEmbedding(context.Background(), "Photovoltaik & Speicher")
	if err != nil {
		t.Fatalf("demo embedding stub failed on second call: %v", err)
	}
	for i := range vec {
		if vec[i] != again[i] {
			t.Fatalf("demo embedding must be deterministic, differs at index %d", i)
		}
	}
	if err := gw.ValidateEmbeddingModel(context.Background()); err != nil {
		t.Fatalf("demo embedding must pass the schema self-check without a backend, got: %v", err)
	}
}

func TestDemoGatewayReportsMarkerAsModelName(t *testing.T) {
	gw := demoGateway(t)

	if got := gw.GetConfig().OllamaModel; !strings.Contains(got, "DEMO") {
		t.Fatalf("demo gateway must announce the stub as its model, got: %q", got)
	}

	obs := ai.NewObservabilityService()
	svc := ai.NewTriageService(gw, obs)
	if _, err := svc.TriageEmail(context.Background(), "a@b.de", "Betreff", "Text"); err != nil {
		t.Fatalf("demo triage failed: %v", err)
	}
	logs := obs.GetRecentLogs(10)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if !strings.Contains(logs[0].ModelName, "DEMO") {
		t.Fatalf("audit log entry must carry the DEMO marker, got ModelName=%q", logs[0].ModelName)
	}
}

func TestDemoResearchStubNeedsNoNetworkAndIsMarked(t *testing.T) {
	gw := demoGateway(t)
	svc := ai.NewResearchService(gw, nil)

	first, err := svc.ResearchCompany(context.Background(), "energie-dach.de")
	if err != nil {
		t.Fatalf("demo research stub failed: %v", err)
	}
	second, err := svc.ResearchCompany(context.Background(), "energie-dach.de")
	if err != nil {
		t.Fatalf("demo research stub failed on second call: %v", err)
	}
	if !strings.Contains(first.Summary, ai.DemoStubMarker) {
		t.Fatalf("demo research summary must carry the marker, got: %q", first.Summary)
	}
	if first.Summary != second.Summary || first.Title != second.Title || strings.Join(first.IndustryKeywords, "|") != strings.Join(second.IndustryKeywords, "|") {
		t.Fatal("demo research stub must be deterministic for identical input")
	}
	if len(first.IndustryKeywords) == 0 {
		t.Fatal("demo research stub must provide keywords for the UI")
	}
}

// Safety property: without demo mode the stub is unreachable. The gateway must
// still contact the configured provider and must still fail exactly the way it
// used to — same error, same wrapped provider message.
func TestStubUnreachableWithoutDemoModeStillCallsProviderAndFailsAsBefore(t *testing.T) {
	// Even a DEMO_MODE environment variable must not activate the stub on its
	// own; only the explicitly wired DemoMode flag may. This keeps the stub
	// away from production gateways that never opted in.
	t.Setenv("DEMO_MODE", "true")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "upstream failure", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "gemma4:12b",
	})

	out, err := gw.Generate(context.Background(), "Hallo", demoChatSystem)
	if hits == 0 {
		t.Fatal("non-demo gateway must still contact the configured provider even when DEMO_MODE is set in the environment")
	}
	if err == nil {
		t.Fatalf("non-demo gateway must still fail the way it used to, got: %q", out)
	}
	if !errors.Is(err, ai.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got: %v", err)
	}
	const wantMsg = "KI-Dienst nicht erreichbar: ollama backend returned HTTP 500"
	if err.Error() != wantMsg {
		t.Fatalf("error must be byte-identical to the pre-stub failure, got: %q want %q", err.Error(), wantMsg)
	}
	if out != "" {
		t.Fatalf("expected empty output on provider failure, got: %q", out)
	}

	// And on success it must return the provider's bytes, not stub text.
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "Echte Modellantwort ohne Demo"})
	}))
	t.Cleanup(okSrv.Close)

	live := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   okSrv.URL,
		OllamaModel:     "gemma4:12b",
	})
	got, err := live.Generate(context.Background(), "Hallo", demoChatSystem)
	if err != nil {
		t.Fatalf("expected provider success passthrough, got: %v", err)
	}
	if got != "Echte Modellantwort ohne Demo" {
		t.Fatalf("expected raw provider bytes, got: %q", got)
	}
	if strings.Contains(got, "DEMO") {
		t.Fatal("non-demo gateway must never emit demo markers")
	}
}

// The launcher probe runs pure-constructor mode; it must never be stubbed by
// demo wiring either.
func TestDisableEnvFallbackGatewayIsNeverStubbedByDemoModeFlag(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": "Provider-Antwort"})
	}))
	t.Cleanup(srv.Close)

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:    ai.ProviderOllama,
		OllamaBaseURL:      srv.URL,
		DisableEnvFallback: true,
	})
	got, err := gw.Generate(context.Background(), "Hallo", demoChatSystem)
	if err != nil {
		t.Fatalf("expected provider call in pure mode, got: %v", err)
	}
	if hits == 0 || got != "Provider-Antwort" {
		t.Fatalf("pure-constructor gateway must use the provider, got hits=%d out=%q", hits, got)
	}
}
