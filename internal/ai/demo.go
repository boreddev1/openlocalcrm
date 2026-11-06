// Package-internal demo stub. All user-facing text below is German; "deterministische"
// is correct German, not a misspelling of the English "deterministic".
//
//nolint:misspell // German stub text would trip the English dictionary
package ai

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// DemoStubMarker is the unmistakable label every piece of demo-mode AI output
// carries. It exists because this repository has repeatedly had to delete
// features that looked like real AI capability but were not (see
// internal/core/automation/draft.go, "never creates fake demo accounts"). A
// test asserts the marker on synthesized content and audit entries so the
// stub can never be quietly turned into a believable model.
const DemoStubMarker = "DEMO — deterministische Platzhalterantwort, keine Inferenz"

// DemoStubEmbeddingModelName is reported instead of a real embedding model so
// KB articles indexed in demo mode visibly belong to the stub.
const DemoStubEmbeddingModelName = "DEMO — deterministischer Embedding-Platzhalter, keine Inferenz"

// demoGenerate answers a Generate call in demo mode without any network,
// model, wall clock or map iteration. Output is a pure function of the
// (already PII-sanitized) request, classified by fixed-order substring checks
// on the system instruction of the callers that E2E exercises:
//
//   - ChatService copilot chat       → marked German reply
//   - TriageService email triage     → marked TriageResult JSON
//   - AIDrafter DRAFT_EMAIL step     → marked "Betreff:" email draft
//   - notes synthesis                → marked analysis JSON (ActivityLogDrawer)
//   - parse-bill OCR                 → marked calculator JSON (offer calculator)
//   - ResearchService summary        → marked summary JSON
//
// Everything else — notably the email-template generator, whose honest failure
// live-simulation.spec asserts — returns an ErrUpstreamUnavailable so no
// canned content can masquerade as a model answer.
func (g *Gateway) demoGenerate(prompt, systemInstruction string) (string, error) {
	switch {
	case strings.Contains(systemInstruction, "OpenLocalCRM KI-Vertriebsassistent"):
		return g.demoChatReply(), nil
	case strings.Contains(systemInstruction, "Analysiere die eingehende E-Mail"):
		return g.demoTriageJSON(), nil
	case strings.Contains(systemInstruction, "professioneller Kundenbetreuer"):
		return g.demoDraftEmailText(), nil
	case strings.Contains(systemInstruction, "Vertriebs-Analyst"):
		return g.demoNotesSynthesisJSON(), nil
	case strings.Contains(systemInstruction, "OCR-Parser für Energierechnungen"):
		return g.demoParseBillJSON(), nil
	case strings.Contains(systemInstruction, "B2B-Recherche-Analyst"):
		return g.demoResearchJSON(), nil
	case strings.Contains(systemInstruction, "E-Mail-Vorlagen-Generator"):
		return "", fmt.Errorf("%w: demo-modus erfindet keine E-Mail-Vorlagen (keine Inferenz, kein Provider)", ErrUpstreamUnavailable)
	default:
		return "", fmt.Errorf("%w: demo-stub hat keinen deterministischen Ausgabetyp für diese Anfrage (keine Inferenz)", ErrUpstreamUnavailable)
	}
}

func (g *Gateway) demoChatReply() string {
	return DemoStubMarker + "\n\n" +
		"Der Demo-Copilot führt keine Inferenz aus und erfindet keine CRM-Zahlen. " +
		"Diese Antwort ist ein fester Platzhaltertext für deterministische E2E-Läufe; " +
		"echte KI-Antworten liefert ausschließlich der konfigurierte Provider."
}

func (g *Gateway) demoTriageJSON() string {
	return `{"category":"ANFRAGE","sentiment":"NEUTRAL","priority":"MEDIUM",` +
		`"summary":"` + DemoStubMarker + `",` +
		`"draft_reply":"Sehr geehrte Damen und Herren, Ihre Mitteilung wurde empfangen und an unsere zuständige Fachabteilung weitergeleitet. (` + DemoStubMarker + `)"}`
}

func (g *Gateway) demoDraftEmailText() string {
	return "Betreff: Demo-Erstkontakt — Platzhalterantwort ohne Inferenz\n\n" +
		"Sehr geehrte Damen und Herren,\n\n" +
		DemoStubMarker + "\n\n" +
		"dieser Entwurf entstand in der Demo ohne jedes KI-Modell und ohne Netzzugriff. " +
		"Er dient nur dem Nachweis der Workflow-Kette (Human-in-the-Freigabe → Entwurf → Aufgabe) " +
		"und ersetzt keine echte Modellantwort.\n\n" +
		"Mit freundlichen Grüßen\nIhr OpenLocalCRM Demo-Assistent"
}

func (g *Gateway) demoNotesSynthesisJSON() string {
	return `{"executive_summary":"` + DemoStubMarker + ` — Die Demo-Synthese wertet keine Notizen aus.",` +
		`"buying_intent":"NICHT ERHOBEN (Demo-Platzhalter, keine Inferenz)",` +
		`"sentiment":"NEUTRAL",` +
		`"key_objections":"` + DemoStubMarker + `",` +
		`"suggested_actions":[{"type":"CREATE_TODO","label":"Demo-Platzhalter-Aufgabe (keine Inferenz)","due_date":"2026-08-28"},` +
		`{"type":"CREATE_DEAL","label":"Demo-Platzhalter-Deal (keine Inferenz)","value":"0.00"}]}`
}

func (g *Gateway) demoParseBillJSON() string {
	return `{"customer_name":"DEMO-Kunde — deterministische Platzhalterantwort, keine Inferenz",` +
		`"yearly_consumption":6500,"current_electricity_price":0.385,` +
		`"meter_number":"1EMH-DEMO-STUB-0000000",` +
		`"roof_area_sqm":75,"roof_orientation":"Süd (35° Dachneigung) — DEMO-Platzhalter",` +
		`"recommended_kwp":14.5,"recommended_storage_kwh":12.0,` +
		`"demo_note":"` + DemoStubMarker + `"}`
}

func (g *Gateway) demoResearchJSON() string {
	return `{"summary":"` + DemoStubMarker + ` — Die Demo-Recherche wertet keine Webseite aus.",` +
		`"industry_keywords":["DEMO-Platzhalter","keine Inferenz"]}`
}

// demoEmbedding returns a deterministic pseudo-vector for demo mode. It is a
// pure function of the (sanitized) text: an SHA-256 chain expanded to the
// fixed schema width. It carries zero semantic meaning — a marker is attached
// where the vector is user-visible (the embedding-model column) — and it
// exists so KB indexing and the startup dimension check never reach for a
// network in demo E2E.
func demoEmbedding(text string) []float32 {
	vec := make([]float32, EmbeddingDimensions)
	block := sha256.Sum256([]byte(text))
	for i := 0; i < EmbeddingDimensions; i++ {
		if i > 0 && i%sha256.Size == 0 {
			block = sha256.Sum256(block[:])
		}
		vec[i] = float32(block[i%sha256.Size])/255.0 - 0.5
	}
	return vec
}
