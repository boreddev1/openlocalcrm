package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

const csrfRestartUserID = "11111111-2222-3333-4444-555555555555"

// csrfResultPrefix marks the child's machine-readable answer line. t.Log would be
// swallowed for a passing child, so the helper writes to stdout directly.
const csrfResultPrefix = "RESULT:"

// TestCSRFTokenSurvivesProcessRestart is the bug itself: the HMAC key used to be
// generated per process, so a token minted before a restart validated afterwards
// only by accident. The two "processes" are real child test binaries, each
// installing the secret the way cmd/server/main.go does - nothing in the parent's
// global state can hide a regression.
func TestCSRFTokenSurvivesProcessRestart(t *testing.T) {
	if mode := os.Getenv("CSRF_RESTART_HELPER"); mode != "" {
		runCSRFRestartHelper(t, mode)
		return
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "ed25519.key")
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if writeErr := os.WriteFile(keyPath, priv, 0600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	token := runCSRFRestartChild(t, "mint", keyPath, "")

	if verdict := runCSRFRestartChild(t, "validate", keyPath, token); verdict != "valid" {
		t.Fatalf("token minted before a restart did not survive it: child reported %q for token %q", verdict, token)
	}

	// Negative control: without this, the test would also pass if validation
	// ignored the secret entirely.
	otherPath := filepath.Join(dir, "other.key")
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(other): %v", err)
	}
	if writeErr := os.WriteFile(otherPath, other, 0600); writeErr != nil {
		t.Fatalf("WriteFile(other): %v", writeErr)
	}
	if verdict := runCSRFRestartChild(t, "validate", otherPath, token); verdict != "invalid" {
		t.Fatalf("a token validated under a different signing key: got %q, want %q", verdict, "invalid")
	}
}

// TestSetCSRFSecretAfterUseIsRefused pins first-use-wins: once the ephemeral key
// has been generated, installing a different one is refused instead of swapping the
// HMAC key under live tokens.
func TestSetCSRFSecretAfterUseIsRefused(t *testing.T) {
	if _, err := auth.GenerateCSRFToken(); err != nil {
		t.Fatalf("GenerateCSRFToken: %v", err)
	}
	err := auth.SetCSRFSecret([]byte("0123456789abcdef0123456789abcdef"))
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("SetCSRFSecret after the ephemeral secret was used = %v, want an in-use refusal", err)
	}
	after, err := auth.GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken after refusal: %v", err)
	}
	if !auth.ValidateCSRFToken(after, "") {
		t.Fatal("a refused install must not have disturbed the working ephemeral secret")
	}
}

func runCSRFRestartChild(t *testing.T, mode, keyPath, token string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCSRFTokenSurvivesProcessRestart")
	cmd.Env = append(os.Environ(),
		"CSRF_RESTART_HELPER="+mode,
		"CSRF_RESTART_KEY="+keyPath,
		"CSRF_RESTART_TOKEN="+token,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child %s failed: %v\n%s", mode, err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), csrfResultPrefix); ok {
			return v
		}
	}
	t.Fatalf("child %s printed no %s line:\n%s", mode, csrfResultPrefix, out)
	return ""
}

func runCSRFRestartHelper(t *testing.T, mode string) {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("CSRF_RESTART_KEY"))
	if err != nil {
		t.Fatalf("helper ReadFile: %v", err)
	}
	if err := auth.InstallCSRFSecretFromSigningKey(ed25519.PrivateKey(data)); err != nil {
		t.Fatalf("helper install: %v", err)
	}
	switch mode {
	case "mint":
		token, err := auth.GenerateBoundCSRFToken(csrfRestartUserID)
		if err != nil {
			t.Fatalf("helper mint: %v", err)
		}
		fmt.Println(csrfResultPrefix + token)
	case "validate":
		if auth.ValidateCSRFToken(os.Getenv("CSRF_RESTART_TOKEN"), csrfRestartUserID) {
			fmt.Println(csrfResultPrefix + "valid")
		} else {
			fmt.Println(csrfResultPrefix + "invalid")
		}
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}
