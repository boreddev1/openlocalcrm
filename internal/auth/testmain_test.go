package auth_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the master-encryption-key location for this whole test binary
// to a temporary directory (issue #122).
//
// Before that, any test reaching crypto.EncryptSecret/DecryptSecret — every TOTP
// flow does — resolved the key path from the working directory default and
// generated a real 256-bit AES key as data/secrets.key *inside the package
// directory*. .gitignore hid the file through the `*.key` filename rule only;
// the directory itself is not covered, so a `git add -A` after a test run could
// have committed the key (or any sibling file written next to it).
//
// The key is process-global: crypto caches it after first use, so a per-test
// t.Setenv would only ever be honored by whichever test ran first. Setting the
// environment once here matches that lifetime and keeps every test in the
// package honest about where its secrets go.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "openlocalcrm-auth-testkeys-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth: failed to create temp key directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("SECRETS_KEY_PATH", filepath.Join(dir, "secrets.key")); err != nil {
		fmt.Fprintf(os.Stderr, "auth: failed to set SECRETS_KEY_PATH: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()

	// Guard the invariant itself: the legacy relative default must never appear
	// in the source tree again.
	if _, statErr := os.Stat(filepath.Join("data", "secrets.key")); statErr == nil {
		fmt.Fprintln(os.Stderr, "auth: a test wrote data/secrets.key into the package directory (issue #122)")
		code = 1
	}

	os.RemoveAll(dir)
	os.Exit(code)
}
