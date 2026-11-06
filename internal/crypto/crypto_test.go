package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearKeyEnv unset every variable GetKeyPath consults, so a test sees the
// resolution chain itself and not the ambient CI/developer environment.
func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"SECRETS_KEY_PATH", "SECRET_KEY_PATH", "STORAGE_PATH", "STORAGE_LOCAL_DIR", "XDG_CONFIG_HOME", "APPDATA", "HOME"} {
		t.Setenv(name, "")
	}
	SetMasterKeyForTest(nil)
	t.Cleanup(func() { SetMasterKeyForTest(nil) })
}

func TestCrypto_EncryptDecryptRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "secrets.key")
	t.Setenv("SECRETS_KEY_PATH", keyFile)
	SetMasterKeyForTest(nil)

	original := []byte("top-secret-database-or-email-password-12345")
	encrypted, err := Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatalf("Encrypted string is empty")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(original, decrypted) {
		t.Fatalf("Expected decrypted %q, got %q", string(original), string(decrypted))
	}

	// Verify key file was created with 0600
	fi, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("Failed to stat key file: %v", err)
	}
	perm := fi.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", perm)
	}
}

func TestCrypto_TamperedCiphertext(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "secrets.key")
	t.Setenv("SECRETS_KEY_PATH", keyFile)
	SetMasterKeyForTest(nil)

	original := []byte("secret-value")
	encrypted, err := Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with the ciphertext
	tampered := []byte(encrypted)
	if tampered[len(tampered)-2] == 'A' {
		tampered[len(tampered)-2] = 'B'
	} else {
		tampered[len(tampered)-2] = 'A'
	}

	_, err = Decrypt(string(tampered))
	if err == nil {
		t.Fatalf("Expected decryption error for tampered ciphertext, got nil")
	}
}

func TestGetKeyPath_CanonicalEnvWins(t *testing.T) {
	clearKeyEnv(t)
	canonical := filepath.Join(t.TempDir(), "secrets.key")
	legacy := filepath.Join(t.TempDir(), "legacy.key")
	storage := t.TempDir()

	t.Setenv("SECRETS_KEY_PATH", canonical)
	t.Setenv("SECRET_KEY_PATH", legacy)
	t.Setenv("STORAGE_PATH", storage)

	got, err := GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath: %v", err)
	}
	if got != canonical {
		t.Errorf("expected SECRETS_KEY_PATH to win, got %q", got)
	}
}

func TestGetKeyPath_LegacyAliasStillHonored(t *testing.T) {
	clearKeyEnv(t)
	legacy := filepath.Join(t.TempDir(), "legacy.key")
	t.Setenv("SECRET_KEY_PATH", legacy)

	got, err := GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath: %v", err)
	}
	if got != legacy {
		t.Errorf("expected legacy SECRET_KEY_PATH %q, got %q", legacy, got)
	}
}

// The two compose files and the launcher template all set STORAGE_PATH to a
// directory backed by a persisted volume, so deriving the key from it keeps
// server and worker on one key without any new configuration.
func TestGetKeyPath_DerivesFromStoragePath(t *testing.T) {
	clearKeyEnv(t)
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	got, err := GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath: %v", err)
	}
	want := filepath.Join(storage, "keys", defaultKeyFileName)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}

	// The documented alias for the storage root resolves to the same place.
	t.Setenv("STORAGE_PATH", "")
	t.Setenv("STORAGE_LOCAL_DIR", storage)
	got, err = GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath with STORAGE_LOCAL_DIR: %v", err)
	}
	if got != want {
		t.Errorf("expected %q via STORAGE_LOCAL_DIR, got %q", want, got)
	}
}

func TestGetKeyPath_FallsBackToUserConfigDir(t *testing.T) {
	clearKeyEnv(t)
	home := t.TempDir()
	// os.UserConfigDir is platform specific: XDG_CONFIG_HOME on Linux,
	// %APPDATA% on Windows, $HOME/Library/Application Support on macOS.
	// Pointing all of them at the sandbox makes the assertion portable.
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)

	got, err := GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected an absolute path, got %q", got)
	}
	if !strings.HasPrefix(filepath.Dir(got), home) {
		t.Errorf("expected %q to live under the user config dir %q", got, home)
	}
	if filepath.Base(got) != defaultKeyFileName {
		t.Errorf("expected file name %q, got %q", defaultKeyFileName, filepath.Base(got))
	}
}

// With nothing configured and no user directory available, the resolution
// fails loudly instead of inventing a path.
func TestGetKeyPath_ErrorWhenUnresolvable(t *testing.T) {
	clearKeyEnv(t)

	got, err := GetKeyPath()
	if err == nil {
		t.Fatalf("expected an error when no key location is resolvable, got %q", got)
	}
	if !strings.Contains(err.Error(), "SECRETS_KEY_PATH") {
		t.Errorf("error should name the variable to set, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty path on error, got %q", got)
	}
}

// Regression test for issue #122: a test (or a developer running the binary
// from a source checkout) must never end up with a generated master key
// written relative to the working directory.
func TestGetOrCreateMasterKey_NeverWritesIntoWorkingDirectory(t *testing.T) {
	clearKeyEnv(t)

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected a 32-byte key, got %d bytes", len(key))
	}

	legacyInTree := filepath.Join(cwd, "data", defaultKeyFileName)
	if _, statErr := os.Stat(legacyInTree); statErr == nil {
		t.Errorf("master key was written into the working directory at %s (issue #122)", legacyInTree)
	}

	keyPath, err := GetKeyPath()
	if err != nil {
		t.Fatalf("GetKeyPath after generating: %v", err)
	}
	if !strings.HasPrefix(keyPath, home) {
		t.Errorf("expected key under %q, got %q", home, keyPath)
	}
	fi, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("expected the key file at %q: %v", keyPath, err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}
}

// The STORAGE_PATH derivation is what the containers use, so the key has to be
// created there and survive a process restart (i.e. stay readable).
func TestGetOrCreateMasterKey_PersistedViaStoragePath(t *testing.T) {
	clearKeyEnv(t)

	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	first, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}

	// Simulate a restart: drop the cache, read again from disk.
	SetMasterKeyForTest(nil)
	second, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey after restart: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("key changed across a simulated restart: master keys would invalidate stored TOTP/email secrets")
	}

	keyPath := filepath.Join(storage, "keys", defaultKeyFileName)
	if fi, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key at %q: %v", keyPath, err)
	} else if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 on %s, got %o", keyPath, perm)
	}
}

// GetOrCreateMasterKey must report the resolution failure instead of writing
// somewhere it invented.
func TestGetOrCreateMasterKey_ReportsUnresolvablePath(t *testing.T) {
	clearKeyEnv(t)

	_, err := GetOrCreateMasterKey()
	if err == nil {
		t.Fatal("expected an error when the key location cannot be resolved")
	}
	if !strings.Contains(err.Error(), "SECRETS_KEY_PATH") {
		t.Errorf("expected the error to name SECRETS_KEY_PATH, got: %v", err)
	}
}
