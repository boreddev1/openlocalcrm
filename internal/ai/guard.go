package ai

import (
	"regexp"
	"strings"
)

var (
	// Regex for IBANs (e.g. DE89370400440532013000)
	ibanRegex = regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}(?:[ ]?[0-9]{4}){4,7}(?:[ ]?[0-9]{1,2})?\b`)
	// Regex for Credit Cards
	creditCardRegex = regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b`)
	// Regex for German Tax ID (Steuer-ID / 11 digits)
	taxIDRegex = regexp.MustCompile(`\b\d{11}\b`)
	// Regex for German VAT ID (USt-IdNr. e.g. DE123456789)
	vatIDRegex = regexp.MustCompile(`\bDE[0-9]{9}\b`)
	// Regex for sensitive passwords or API keys
	secretKeyRegex = regexp.MustCompile(`(?i)(api[_-]?key|password|secret|token)\s*[:=]\s*['"]?([a-zA-Z0-9_\-\.]{12,})['"]?`)
	// Regex for script tags / javascript execution injection
	scriptTagRegex = regexp.MustCompile(`(?i)<script[\s\S]*?>[\s\S]*?<\/script>|<script[\s\S]*?>|javascript:`)
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

	// Finding #4: Steuer-ID & USt-IdNr Maskierung
	count += len(taxIDRegex.FindAllStringIndex(sanitized, -1))
	sanitized = taxIDRegex.ReplaceAllString(sanitized, "[REDACTED_TAX_ID]")

	count += len(vatIDRegex.FindAllStringIndex(sanitized, -1))
	sanitized = vatIDRegex.ReplaceAllString(sanitized, "[REDACTED_VAT_ID]")

	count += len(secretKeyRegex.FindAllStringIndex(sanitized, -1))
	sanitized = secretKeyRegex.ReplaceAllString(sanitized, "$1: [REDACTED_SECRET]")

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

	return clean, nil
}
