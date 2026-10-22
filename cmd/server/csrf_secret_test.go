package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installCSRFSecret is the startup path that keeps CSRF tokens alive across a
// restart. Every failure here is fatal in main(), so each one is pinned.

func signingKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv
}

func TestInstallCSRFSecretFailClosedOnConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		write   bool
		want    string
	}{
		{"unreadable path", "", false, "unreadable"},
		{"empty file", "", true, "empty"},
		{"whitespace only", "\n\t \n", true, "empty"},
		{"too short", "abc", true, "too short"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_"))
			if tc.write {
				if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			t.Setenv("CSRF_SECRET_KEY_PATH", path)
			err := installCSRFSecret(signingKey(t))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("installCSRFSecret() = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestInstallCSRFSecretAcceptsOperatorFileWithNewline(t *testing.T) {
	secret := strings.Repeat("f", 32)
	path := filepath.Join(t.TempDir(), "csrf.key")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("CSRF_SECRET_KEY_PATH", path)
	if err := installCSRFSecret(signingKey(t)); err != nil {
		t.Fatalf("a 32-byte secret with a trailing newline must install, got %v", err)
	}
}

func TestInstallCSRFSecretDerivesWithoutConfiguration(t *testing.T) {
	t.Setenv("CSRF_SECRET_KEY_PATH", "")
	key := signingKey(t)
	err := installCSRFSecret(key)
	if err == nil || !strings.Contains(err.Error(), "in use") && !strings.Contains(err.Error(), "already") {
		t.Fatalf("installCSRFSecret() = %v; want success only if nothing used the secret yet, otherwise a refusal that proves first-use-wins", err)
	}
	// Whichever branch ran, a second install must be refused, never silently accepted.
	if err := installCSRFSecret(key); err == nil {
		t.Fatal("a second install succeeded; the CSRF key can be swapped under live tokens")
	}
}
