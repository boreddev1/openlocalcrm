package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

// Issue #82: the PII guard redacted IBAN, credit cards, tax IDs, VAT IDs, secrets
// and script tags — but not e-mail addresses. A live model echoed "meier@gmbh.de"
// in 1 of 3 triage runs. These deterministic tests lock the two gaps: SanitizeInput
// must redact mail, ValidateOutput must reject a mail leak.

func TestGuardSanitizeInputRedactsEmail(t *testing.T) {
	guard := ai.NewGuard()

	cases := []struct {
		name  string
		input string
	}{
		{"plain", "Bitte melden Sie sich unter meier@gmbh.de danke."},
		{"mailto scheme", "Kontakt: mailto:meier@gmbh.de"},
		{"upper case", "Erreichbar unter MEIER@GMBH.DE jederzeit."},
		{"obfuscated spaces around at", "Schreiben Sie an meier @ gmbh.de bitte."},
		{"subdomain and plus tag", "x.y+z@mail.example.co.uk ist privat."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized, count := guard.SanitizeInputWithCount(tc.input)
			if count < 1 {
				t.Fatalf("expected at least one redaction, got %d for %q", count, tc.input)
			}
			if !strings.Contains(sanitized, "[REDACTED_EMAIL]") {
				t.Fatalf("expected [REDACTED_EMAIL] marker, got: %s", sanitized)
			}
			// The local part and the full domain must not survive anywhere.
			for _, fragment := range []string{"meier", "gmbh.de", "GMBH.DE", "x.y+z", "example.co.uk"} {
				if strings.Contains(strings.ToLower(sanitized), strings.ToLower(fragment)) {
					t.Fatalf("email fragment %q leaked after redaction: %s", fragment, sanitized)
				}
			}
		})
	}
}

func TestGuardSanitizeInputDoesNotFalsePositiveOnEmail(t *testing.T) {
	guard := ai.NewGuard()

	// None of these must be treated as an e-mail address.
	cases := []struct {
		name  string
		input string
	}{
		{"IBAN has no at sign", "Zahlung an DE89370400440532013000 bitte"},
		{"bare comparison with at", "Wenn a @ b dann c"},
		{"no TLD domain", "ping user@localhost"},
		{"plain sentence", "Es ist 5 @ 3 = 15"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized, _ := guard.SanitizeInputWithCount(tc.input)
			if strings.Contains(sanitized, "[REDACTED_EMAIL]") {
				t.Fatalf("false-positive email redaction: %s", sanitized)
			}
		})
	}
}

// The IBAN false-positive guard from the issue: a mail pattern must never disturb
// the existing IBAN redaction.
func TestGuardEmailRedactionLeavesIBANUntouchedAsEmail(t *testing.T) {
	guard := ai.NewGuard()
	sanitized, _ := guard.SanitizeInputWithCount("Rechnung: DE89370400440532013000")
	if strings.Contains(sanitized, "[REDACTED_EMAIL]") {
		t.Fatalf("IBAN must not be classified as email: %s", sanitized)
	}
	if !strings.Contains(sanitized, "[REDACTED_IBAN]") {
		t.Fatalf("IBAN must still be redacted: %s", sanitized)
	}
}

func TestGuardValidateOutputRejectsEmailLeak(t *testing.T) {
	guard := ai.NewGuard()

	leaks := []string{
		"Der Kunde ist unter meier@gmbh.de erreichbar.",
		"Antwort an MEIER@GMBH.DE gesendet.",
		"Bitte mailto:meier@gmbh.de verwenden.",
	}
	for _, out := range leaks {
		if _, err := guard.ValidateOutput(out); err == nil {
			t.Fatalf("expected ValidateOutput to reject email leak, got nil for: %q", out)
		}
	}
}

func TestGuardValidateOutputAllowsCleanOutput(t *testing.T) {
	guard := ai.NewGuard()

	clean := []string{
		"{\"summary\":\"Kundenanfrage erfasst\",\"category\":\"ANFRAGE\"}",
		"Mit freundlichen Grüßen, Ihr Team.",
		"Der Kunde möchte ein Angebot für eine PV-Anlage.",
	}
	for _, out := range clean {
		cleaned, err := guard.ValidateOutput(out)
		if err != nil {
			t.Fatalf("clean output must not be rejected: %q -> %v", out, err)
		}
		if cleaned == "" {
			t.Fatalf("clean output must be preserved, got empty for %q", out)
		}
	}
}

// The triage path must sanitize the body before it ever leaves the process. The
// sender is passed separately to TriageEmail, so redacting the mail address inside
// the body costs the model no information it needs to classify.
func TestTriagePathSanitizesBodyBeforeSend(t *testing.T) {
	var got io.Reader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = strings.NewReader(string(body))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"response": `{"category":"ANFRAGE","sentiment":"POSITIVE","priority":"HIGH","summary":"Angebot erbeten.","draft_reply":"Gerne."}`})
	}))
	t.Cleanup(srv.Close)

	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
	svc := ai.NewTriageService(gw)

	_, err := svc.TriageEmail(
		context.Background(),
		"m.meier@gmbh.de",
		"Anfrage Angebot",
		"Sehr geehrte Damen, meier@gmbh.de, Telefon 0171 2345678, bitte Angebot senden.",
	)
	if err != nil {
		t.Fatalf("triage failed: %v", err)
	}

	raw, _ := io.ReadAll(got)
	req := string(raw)
	if strings.Contains(req, "meier@gmbh.de") {
		t.Fatalf("raw email reached the model provider: %s", req)
	}
	if !strings.Contains(req, "[REDACTED_EMAIL]") {
		t.Fatalf("expected body email redacted before send, got: %s", req)
	}
}
