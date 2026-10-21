package ai

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrPIILeakDetected signals that a model response still contained unredacted PII
// after generation and must never be surfaced to the caller. It is returned by
// ValidateOutput as the second, output-side guard on top of input sanitization.
var ErrPIILeakDetected = errors.New("PII-Leak in der KI-Ausgabe erkannt")

var (
	// Regex for IBANs (e.g. DE89370400440532013000)
	ibanRegex = regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}(?:[ ]?[0-9]{4}){4,7}(?:[ ]?[0-9]{1,2})?\b`)
	// Regex for Credit Cards
	creditCardRegex = regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b`)
	// Regex for German Tax ID (Steuer-ID / IdNr / TIN) requiring contextual prefix or standard 2-3-3-3 formatting (F-48)
	taxIDPrefixedRegex  = regexp.MustCompile(`(?i)\b(?:steuer[- ]?id(?:nr)?|idnr|tin|steuer[- ]?identifikationsnummer|steuernummer)(?:[\s:=]+(?:ist|lautet|nr|nummer|:|-)?)*[\s:=]+(?:\d{2}[ -]?\d{3}[ -]?\d{3}[ -]?\d{3}|\d{11})\b`)
	taxIDFormattedRegex = regexp.MustCompile(`\b\d{2}\s\d{3}\s\d{3}\s\d{3}\b`)
	// Regex for German VAT ID (USt-IdNr. e.g. DE123456789)
	vatIDRegex = regexp.MustCompile(`\bDE[0-9]{9}\b`)
	// Regex for sensitive passwords or API keys
	secretKeyRegex = regexp.MustCompile(`(?i)(api[_-]?key|password|secret|token)\s*[:=]\s*['"]?([a-zA-Z0-9_\-\.]{12,})['"]?`)
	// Regex for script tags / javascript execution injection
	scriptTagRegex = regexp.MustCompile(`(?i)<script[\s\S]*?>[\s\S]*?<\/script>|<script[\s\S]*?>|javascript:`)
	// Regex for e-mail addresses (issue #82). Optional mailto: scheme, a tolerant
	// local part, optional spaces around the @ to catch "meier @ gmbh.de"
	// obfuscation, and a domain that must end in a dotted TLD. The TLD requirement
	// is what keeps "user@localhost", "5 @ 3 = 15" and IBANs from matching.
	mailRegex = regexp.MustCompile(`(?i)(?:mailto:)?[A-Za-z0-9._%+\-]+[ \t]*@[ \t]*[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
)

type Guard struct{}

func NewGuard() *Guard {
	return &Guard{}
}

// SanitizeInputWithCount redacts sensitive PII before sending context to LLMs and returns the number of redactions
func (g *Guard) SanitizeInputWithCount(text string) (string, int) {
	if text == "" {
		return text, 0
	}

	count := 0
	count += len(ibanRegex.FindAllStringIndex(text, -1))
	sanitized := ibanRegex.ReplaceAllString(text, "[REDACTED_IBAN]")

	count += len(creditCardRegex.FindAllStringIndex(sanitized, -1))
	sanitized = creditCardRegex.ReplaceAllString(sanitized, "[REDACTED_CREDIT_CARD]")

	// Finding #4 / F-48: Steuer-ID & USt-IdNr Maskierung mit Kontextprüfung
	count += len(taxIDPrefixedRegex.FindAllStringIndex(sanitized, -1))
	sanitized = taxIDPrefixedRegex.ReplaceAllString(sanitized, "[REDACTED_TAX_ID]")

	count += len(taxIDFormattedRegex.FindAllStringIndex(sanitized, -1))
	sanitized = taxIDFormattedRegex.ReplaceAllString(sanitized, "[REDACTED_TAX_ID]")

	count += len(vatIDRegex.FindAllStringIndex(sanitized, -1))
	sanitized = vatIDRegex.ReplaceAllString(sanitized, "[REDACTED_VAT_ID]")

	count += len(secretKeyRegex.FindAllStringIndex(sanitized, -1))
	sanitized = secretKeyRegex.ReplaceAllString(sanitized, "$1: [REDACTED_SECRET]")

	// Issue #82: redact e-mail addresses. The triage sender is passed as its own
	// argument, so redacting the mail inside the body costs the model no
	// information it needs to classify.
	count += len(mailRegex.FindAllStringIndex(sanitized, -1))
	sanitized = mailRegex.ReplaceAllString(sanitized, "[REDACTED_EMAIL]")

	return sanitized, count
}

// SanitizeInput redacts sensitive PII before sending context to LLMs
func (g *Guard) SanitizeInput(text string) string {
	sanitized, _ := g.SanitizeInputWithCount(text)
	return sanitized
}

// ValidateOutput ensures that LLM responses do not echo forbidden system instructions or dangerous script injections
func (g *Guard) ValidateOutput(output string) (string, error) {
	clean := strings.TrimSpace(output)

	// Remove markdown codeblock fences if present for raw JSON parsing
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	// Finding #10: Check for dangerous script injection
	if scriptTagRegex.MatchString(clean) {
		clean = scriptTagRegex.ReplaceAllString(clean, "[BLOCKED_SCRIPT]")
	}

	// Finding #10: Check for system prompt instruction leak
	leakSignatures := []string{
		"System Instruction:",
		"<untrusted_user_context>",
		"</untrusted_user_context>",
	}
	for _, sig := range leakSignatures {
		if strings.Contains(clean, sig) {
			clean = strings.ReplaceAll(clean, sig, "[REDACTED_INTERNAL]")
		}
	}

	// Issue #82: reject any mail address that survived to the output. Input
	// sanitization is the primary redaction; this is the output-side guard that
	// refuses to release a leak even if a model reconstructs or echoes one.
	if mailRegex.MatchString(clean) {
		return "", fmt.Errorf("%w: unredacted e-mail in model output", ErrPIILeakDetected)
	}

	return clean, nil
}
