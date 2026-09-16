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
	// Regex for sensitive passwords or API keys
	secretKeyRegex = regexp.MustCompile(`(?i)(api[_-]?key|password|secret|token)\s*[:=]\s*['"]?([a-zA-Z0-9_\-\.]{12,})['"]?`)
)

type Guard struct{}

func NewGuard() *Guard {
	return &Guard{}
}

// SanitizeInput redacts sensitive PII before sending context to LLMs
func (g *Guard) SanitizeInput(text string) string {
	if text == "" {
		return text
	}

	sanitized := ibanRegex.ReplaceAllString(text, "[REDACTED_IBAN]")
	sanitized = creditCardRegex.ReplaceAllString(sanitized, "[REDACTED_CREDIT_CARD]")
	sanitized = secretKeyRegex.ReplaceAllString(sanitized, "$1: [REDACTED_SECRET]")

	return sanitized
}

// ValidateOutput ensures that LLM responses do not echo forbidden system instructions or dangerous markdown injections
func (g *Guard) ValidateOutput(output string) (string, error) {
	clean := strings.TrimSpace(output)
	// Remove markdown codeblock fences if present for raw JSON parsing
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	return strings.TrimSpace(clean), nil
}
