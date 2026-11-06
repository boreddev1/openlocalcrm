package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// clearKeyEnv unset every variable GetKeyPath consults and sandbox the parts of
// the resolution chain that look at the real filesystem, so a test sees the
// chain itself and not the ambient CI/developer machine.
//
// The legacy-rescue step probes two locations: the working directory (which the
// caller controls with t.Chdir) and the documented container path. Redirecting
// the latter into a directory that never exists keeps a stray
// /app/data/secrets.key - which a container built from the pre-#122 image can
// really have - from changing the outcome of an unrelated assertion.
func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"SECRETS_KEY_PATH", "SECRET_KEY_PATH", "STORAGE_PATH", "STORAGE_LOCAL_DIR", "XDG_CONFIG_HOME", "APPDATA", "HOME"} {
		t.Setenv(name, "")
	}
	SetMasterKeyForTest(nil)
	t.Cleanup(func() { SetMasterKeyForTest(nil) })

	realContainerPath := legacyKeyContainerPath
	legacyKeyContainerPath = filepath.Join(t.TempDir(), "sandbox-app", "data", defaultKeyFileName)
	// Cleanup functions run LIFO, so this restores the production value after
	// the test body has finished overwriting it again (see
	// TestLegacyKeyCandidates).
	t.Cleanup(func() { legacyKeyContainerPath = realContainerPath })

	resetKeyNoticeForTest()
	t.Cleanup(resetKeyNoticeForTest)

	// Precondition for the assertions below: neither probe location of the
	// legacy-rescue step may hold a key, otherwise the rescue legitimately
	// points at it and every "expected a freshly minted key" assertion becomes
	// meaningless. A leftover in the package directory is the pre-#122 artifact
	// from issue #122 - delete it and re-run.
	if cwd, err := os.Getwd(); err == nil {
		if stray := filepath.Join(cwd, legacyKeyRelPath); regularFileExists(stray) {
			t.Fatalf("a pre-#122 master key exists at %s; delete it (issue #122) before running these tests", stray)
		}
	}
	if regularFileExists(legacyKeyContainerPath) {
		t.Fatalf("the sandboxed container key location %s exists; clearKeyEnv did not sandbox it", legacyKeyContainerPath)
	}
}

// useRealContainerCandidate points the container probe back at
// /app/data/secrets.key for the tests that are about the candidate list
// itself. It never asserts and never creates anything, so it stays correct on
// hosts where /app does not exist (the normal case: the file is only ever there
// in a container built from the pre-#122 image).
func useRealContainerCandidate(t *testing.T) {
	t.Helper()
	sandboxed := legacyKeyContainerPath
	legacyKeyContainerPath = "/app/data/secrets.key"
	t.Cleanup(func() { legacyKeyContainerPath = sandboxed })
}

// writeKeyFile puts a deterministic 32-byte master key at path and returns it.
func writeKeyFile(t *testing.T, path string, seed byte) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed + byte(i)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return key
}

// readRaw reads a file without failing on absence, for "must not have been
// created / must not have been touched" assertions.
func readRaw(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}

// mustRead reads a file or fails the test.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// legacyPathIn returns the pre-#122 cwd-relative location under dir.
func legacyPathIn(dir string) string {
	return filepath.Join(dir, legacyKeyRelPath)
}

// derivedPathIn returns the current default location for a storage root.
func derivedPathIn(storage string) string {
	return filepath.Join(storage, "keys", defaultKeyFileName)
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

// --- fail-closed: a present but unusable key is never replaced (issue #128) --

// listDir names the entries of a directory, for "nothing else was written"
// assertions: no second key, no temp-file residue.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// (a) No key file yet: the key is minted at the resolved location with 0600
// and published without leaving temporary files behind.
func TestGetOrCreateMasterKey_CreatesWhenAbsent(t *testing.T) {
	clearKeyEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, defaultKeyFileName)
	t.Setenv("SECRETS_KEY_PATH", path)

	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected a 32-byte key, got %d bytes", len(key))
	}
	if after := mustRead(t, path); !bytes.Equal(after, key) {
		t.Error("the key on disk is not the key in use")
	}
	if fi, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	} else if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 on %s, got %o", path, perm)
	}
	if names := listDir(t, dir); len(names) != 1 || names[0] != defaultKeyFileName {
		t.Errorf("expected only the key file after minting, got %v", names)
	}
}

// (b) An existing valid key file is reused exactly: the returned bytes match,
// the file content (hash) is unchanged, and the directory gains no other
// entry. Regenerating here would orphan every stored secret - the #128 bug.
func TestGetOrCreateMasterKey_ReusesExistingValidKey(t *testing.T) {
	clearKeyEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, defaultKeyFileName)
	want := writeKeyFile(t, path, 7)
	before := sha256Of(t, mustRead(t, path))
	t.Setenv("SECRETS_KEY_PATH", path)

	got, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("an existing valid key was replaced instead of reused - stored secrets would be undecryptable")
	}
	if after := mustRead(t, path); sha256Of(t, after) != before {
		t.Error("the key file content changed on a read-only reuse")
	}
	if names := listDir(t, dir); len(names) != 1 || names[0] != defaultKeyFileName {
		t.Errorf("expected the directory to hold only the key file, got %v", names)
	}

	// And after a restart (cache dropped) the same file is still the key.
	SetMasterKeyForTest(nil)
	again, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey after restart: %v", err)
	}
	if !bytes.Equal(again, want) {
		t.Error("the key changed across a simulated restart")
	}
	if after := mustRead(t, path); sha256Of(t, after) != before {
		t.Error("the restart rewrote the key file")
	}
}

// sha256Of hashes content so "unchanged" assertions are explicit rather than
// relying on bytes.Equal alone.
func sha256Of(t *testing.T, content []byte) string {
	t.Helper()
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// (c) The key file exists but is not a usable key - empty, truncated, wrong
// byte length, garbage, base64 of the wrong length. Every case is a hard
// error naming the path and the cause, and the file bytes on disk are
// provably UNCHANGED afterwards: no truncation, no overwrite, no fresh key,
// no temp residue, and the failure is not cached away - the next call (and
// Encrypt/Decrypt through it) keeps failing until the operator fixes it.
func TestGetOrCreateMasterKey_FailClosedOnUnusableFile(t *testing.T) {
	realKey := make([]byte, 32)
	for i := range realKey {
		realKey[i] = byte(i)
	}

	tests := []struct {
		name      string
		content   []byte
		wantCause string
	}{
		{name: "empty file", content: []byte(""), wantCause: "empty file, 0 bytes"},
		{name: "truncated key", content: realKey[:31], wantCause: "looks truncated"},
		{name: "short garbage", content: []byte("kurz"), wantCause: "does not contain a usable key"},
		{name: "long garbage", content: bytes.Repeat([]byte("x"), 50), wantCause: "does not contain a usable key"},
		{name: "base64 of wrong length", content: []byte(base64.StdEncoding.EncodeToString(realKey[:16])), wantCause: "does not contain a usable key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearKeyEnv(t)

			dir := t.TempDir()
			path := filepath.Join(dir, defaultKeyFileName)
			if err := os.WriteFile(path, tc.content, 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			t.Setenv("SECRETS_KEY_PATH", path)

			key, err := GetOrCreateMasterKey()
			if err == nil {
				t.Fatalf("expected a hard error for an unusable key file, got a key %x", key)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the error must name the path, got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantCause) {
				t.Errorf("the error must name the cause %q, got: %v", tc.wantCause, err)
			}
			if !strings.Contains(err.Error(), "Refusing to overwrite") {
				t.Errorf("the error must say the file will not be touched, got: %v", err)
			}
			if !errors.Is(err, ErrKeyUnusable) {
				t.Errorf("the error must classify as ErrKeyUnusable for callers, got: %v", err)
			}
			// Non-vacuous core assertion: the bytes on disk did not move.
			if after := mustRead(t, path); !bytes.Equal(after, tc.content) {
				t.Errorf("the invalid key file was rewritten: got %x, want %x", after, tc.content)
			}
			if names := listDir(t, dir); len(names) != 1 || names[0] != defaultKeyFileName {
				t.Errorf("expected only the untouched key file, got %v", names)
			}
			// The failure must not be papered over by a cached mint:
			SetMasterKeyForTest(nil)
			if _, err2 := GetOrCreateMasterKey(); err2 == nil {
				t.Error("a second call succeeded - a fresh key replaced the broken one")
			}
			// ...and encryption through the same code path refuses too.
			if _, err := EncryptSecret("vertrieb@openlocalcrm.local"); err == nil {
				t.Error("EncryptSecret succeeded over a broken master key")
			} else if !errors.Is(err, ErrKeyUnusable) {
				t.Errorf("EncryptSecret must propagate ErrKeyUnusable, got: %v", err)
			}
		})
	}
}

// The fail-closed rule covers every NON-legacy location named in the issue:
// SECRETS_KEY_PATH, its SECRET_KEY_PATH alias, $STORAGE_PATH/keys/secrets.key
// and the user-config-dir location.
func TestGetOrCreateMasterKey_FailClosedAtEveryNonLegacyLocation(t *testing.T) {
	garbage := bytes.Repeat([]byte("x"), 50)

	cases := []struct {
		name  string
		setup func(t *testing.T) (path string, dir string)
	}{
		{
			name: "SECRETS_KEY_PATH",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				path := filepath.Join(dir, defaultKeyFileName)
				t.Setenv("SECRETS_KEY_PATH", path)
				return path, dir
			},
		},
		{
			name: "SECRET_KEY_PATH alias",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				path := filepath.Join(dir, defaultKeyFileName)
				t.Setenv("SECRET_KEY_PATH", path)
				return path, dir
			},
		},
		{
			name: "STORAGE_PATH derived",
			setup: func(t *testing.T) (string, string) {
				storage := t.TempDir()
				t.Setenv("STORAGE_PATH", storage)
				path := derivedPathIn(storage)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return path, filepath.Dir(path)
			},
		},
		{
			name: "user config dir",
			setup: func(t *testing.T) (string, string) {
				home := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", home)
				t.Setenv("APPDATA", home)
				t.Setenv("HOME", home)
				path := filepath.Join(home, "openlocalcrm", defaultKeyFileName)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return path, filepath.Dir(path)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearKeyEnv(t)
			path, dir := tc.setup(t)
			if err := os.WriteFile(path, garbage, 0600); err != nil {
				t.Fatalf("write: %v", err)
			}

			_, err := GetOrCreateMasterKey()
			if err == nil {
				t.Fatal("expected a hard error instead of a regenerated key")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the error must name %s, got: %v", path, err)
			}
			if !errors.Is(err, ErrKeyUnusable) {
				t.Errorf("expected ErrKeyUnusable, got: %v", err)
			}
			if after := mustRead(t, path); !bytes.Equal(after, garbage) {
				t.Error("the invalid key file was overwritten")
			}
			if names := listDir(t, dir); len(names) != 1 || names[0] != defaultKeyFileName {
				t.Errorf("expected only the untouched key file in %s, got %v", dir, names)
			}
		})
	}
}

// (d) A key file that exists but cannot be read (permissions) is "present but
// unusable", not "absent": hard error, file untouched, nothing generated.
func TestGetOrCreateMasterKey_FailClosedOnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permissions are not enforced")
	}
	clearKeyEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, defaultKeyFileName)
	want := writeKeyFile(t, path, 8)
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	t.Setenv("SECRETS_KEY_PATH", path)

	_, err := GetOrCreateMasterKey()
	if err == nil {
		t.Fatal("expected a hard error for an unreadable key file, got nil")
	}
	if !strings.Contains(err.Error(), "cannot read the master key file") {
		t.Errorf("the error must say the file cannot be read, got: %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error must name the path, got: %v", err)
	}
	if !errors.Is(err, ErrKeyUnusable) {
		t.Errorf("expected ErrKeyUnusable, got: %v", err)
	}
	// Give the test itself read access back and prove the file never moved:
	// a fail-open implementation would have replaced it with a fresh key here.
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatalf("chmod 600: %v", err)
	}
	if after := mustRead(t, path); !bytes.Equal(after, want) {
		t.Error("the unreadable key file was replaced")
	}
	if names := listDir(t, dir); len(names) != 1 || names[0] != defaultKeyFileName {
		t.Errorf("expected only the original key file, got %v", names)
	}
}

// The mint path stays atomic under the real server+worker shape: concurrent
// first use on an EMPTY location must converge on exactly one key - the bytes
// on disk - with no second file and no temp residue. (With the old
// check-write-write code both renames could publish different bytes and the
// two processes would keep encrypting with two different masters.)
func TestGetOrCreateMasterKey_ConcurrentFirstMintConverges(t *testing.T) {
	clearKeyEnv(t)

	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	const n = 8
	keys := make([][]byte, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			keys[i], errs[i] = GetOrCreateMasterKey()
		}(i)
	}
	wg.Wait()

	onDisk := mustRead(t, derivedPathIn(storage))
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if !bytes.Equal(keys[i], onDisk) {
			t.Fatalf("goroutine %d holds a key that is not on disk - split-brain masters", i)
		}
	}
	if names := listDir(t, filepath.Dir(derivedPathIn(storage))); len(names) != 1 || names[0] != defaultKeyFileName {
		t.Errorf("expected exactly one key file, got %v", names)
	}
}

// --- legacy reuse (rescue of pre-#122 installs, read-only) -------------------

// (a) A legacy key exists and nothing else does: reuse it, warn loudly, and do
// not mint a second key. This is the case that would otherwise silently turn
// every stored TOTP seed and IMAP password into undecryptable bytes.
func TestResolveKeyPath_LegacyRescue_OnlyLegacyKeyExists(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	legacyKey := writeKeyFile(t, legacyPathIn(cwd), 11)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("resolveKeyPath: %v", err)
	}
	if res.Source != KeyPathLegacy {
		t.Errorf("expected source %q, got %q", KeyPathLegacy, res.Source)
	}
	if res.Path != legacyPathIn(cwd) {
		t.Errorf("expected to resolve to the existing legacy key %q, got %q", legacyPathIn(cwd), res.Path)
	}
	if res.CreatePath != filepath.Join(home, "openlocalcrm", defaultKeyFileName) {
		t.Errorf("a new key must still be minted at the current default, got CreatePath %q", res.CreatePath)
	}
	if res.IgnoredLegacyPath != "" {
		t.Errorf("the chosen key is the legacy one, nothing should be ignored: %q", res.IgnoredLegacyPath)
	}
	if notice := res.Notice(); notice == "" || !strings.Contains(notice, legacyPathIn(cwd)) {
		t.Errorf("expected a warning naming the legacy path, got %q", notice)
	}

	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(key, legacyKey) {
		t.Fatal("the legacy key was not reused - secrets encrypted with it would be undecryptable")
	}

	// No second key anywhere.
	if _, statErr := readRaw(t, res.CreatePath); !os.IsNotExist(statErr) {
		t.Errorf("a second master key was created at %q (stat err = %v)", res.CreatePath, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(home, "openlocalcrm")); statErr == nil {
		t.Errorf("the user config key directory was created even though the legacy key was used")
	}
	// The legacy file itself was not rewritten.
	if after := mustRead(t, legacyPathIn(cwd)); !bytes.Equal(after, legacyKey) {
		t.Error("the legacy key file changed on disk")
	}

	// Secrets keep round-tripping across a simulated restart, which is the
	// whole point of the rescue.
	sealed, err := EncryptSecret("vertrieb@openlocalcrm.local")
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	SetMasterKeyForTest(nil)
	opened, err := DecryptSecret(sealed)
	if err != nil {
		t.Fatalf("DecryptSecret after restart: %v", err)
	}
	if opened != "vertrieb@openlocalcrm.local" {
		t.Errorf("roundtrip mismatch: %q", opened)
	}
}

// The same rescue with a storage root configured (the container shape: legacy
// key in the image layer, STORAGE_PATH volume still empty).
func TestResolveKeyPath_LegacyRescue_WithStoragePath(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	storage := t.TempDir()
	legacyKey := writeKeyFile(t, legacyPathIn(cwd), 13)
	t.Setenv("STORAGE_PATH", storage)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("resolveKeyPath: %v", err)
	}
	if res.Source != KeyPathLegacy || res.Path != legacyPathIn(cwd) {
		t.Fatalf("expected the legacy rescue, got %+v", res)
	}
	if res.CreatePath != derivedPathIn(storage) {
		t.Errorf("expected CreatePath %q, got %q", derivedPathIn(storage), res.CreatePath)
	}
	if notice := res.Notice(); !strings.Contains(notice, derivedPathIn(storage)) || !strings.Contains(notice, "SECRETS_KEY_PATH") {
		t.Errorf("the warning must name the copy-to location and the pin, got %q", notice)
	}

	t.Chdir(cwd)
	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(key, legacyKey) {
		t.Fatal("expected the legacy key to be reused")
	}
	if _, err := readRaw(t, derivedPathIn(storage)); !os.IsNotExist(err) {
		t.Errorf("a second key was minted in the storage volume (stat err = %v)", err)
	}
}

// (b) An explicitly configured path wins outright over a legacy file - the
// operator's configuration is never second-guessed by the rescue.
func TestResolveKeyPath_ExplicitPathBeatsExistingLegacyKey(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	legacyKey := writeKeyFile(t, legacyPathIn(cwd), 17)
	storage := t.TempDir()

	for _, env := range [][2]string{{"SECRETS_KEY_PATH", ""}, {"SECRET_KEY_PATH", ""}} {
		t.Run(env[0], func(t *testing.T) {
			pinned := filepath.Join(t.TempDir(), "secrets.key")
			t.Setenv("STORAGE_PATH", storage)
			t.Setenv("SECRETS_KEY_PATH", "")
			t.Setenv("SECRET_KEY_PATH", "")
			t.Setenv(env[0], pinned)

			res, err := resolveKeyPath(cwd)
			if err != nil {
				t.Fatalf("resolveKeyPath: %v", err)
			}
			if res.Source != KeyPathExplicit || res.Path != pinned {
				t.Fatalf("expected the explicit path %q to win, got %+v", pinned, res)
			}
			if res.IgnoredLegacyPath != "" {
				t.Errorf("explicit configuration must not probe legacy locations: %+v", res)
			}
			if res.Notice() != "" {
				t.Errorf("no warning is warranted when the operator pinned the path, got %q", res.Notice())
			}

			t.Chdir(cwd)
			key, err := GetOrCreateMasterKey()
			if err != nil {
				t.Fatalf("GetOrCreateMasterKey: %v", err)
			}
			if bytes.Equal(key, legacyKey) {
				t.Fatal("the legacy key was used instead of the pinned path")
			}
			if got := mustRead(t, pinned); !bytes.Equal(got, key) {
				t.Errorf("the pinned key file does not hold the key in use")
			}
			if after := mustRead(t, legacyPathIn(cwd)); !bytes.Equal(after, legacyKey) {
				t.Error("the untouched legacy file was modified")
			}
			if _, err := readRaw(t, derivedPathIn(storage)); !os.IsNotExist(err) {
				t.Errorf("a key was minted in the storage volume as well (stat err = %v)", err)
			}
			SetMasterKeyForTest(nil)
		})
	}
}

// (c) Both a legacy key and a key at the current default location exist. The
// documented, tested tie-break: the current default location wins, the legacy
// file is reported and left alone.
func TestResolveKeyPath_StorageDerivedKeyWinsOverLegacyKey(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	storage := t.TempDir()
	legacyKey := writeKeyFile(t, legacyPathIn(cwd), 19)
	derivedKey := writeKeyFile(t, derivedPathIn(storage), 23)
	t.Setenv("STORAGE_PATH", storage)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("resolveKeyPath: %v", err)
	}
	if res.Source != KeyPathStorage || res.Path != derivedPathIn(storage) {
		t.Fatalf("expected the storage-derived key to win, got %+v", res)
	}
	if res.IgnoredLegacyPath != legacyPathIn(cwd) {
		t.Errorf("expected the legacy file to be reported as ignored, got %q", res.IgnoredLegacyPath)
	}
	notice := res.Notice()
	if !strings.Contains(notice, derivedPathIn(storage)) || !strings.Contains(notice, legacyPathIn(cwd)) {
		t.Errorf("the tie-break warning must name both files, got %q", notice)
	}
	if !strings.Contains(notice, "SECRETS_KEY_PATH") {
		t.Errorf("the tie-break warning must name the pin, got %q", notice)
	}

	t.Chdir(cwd)
	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(key, derivedKey) {
		t.Fatal("secrets written since the fix would be undecryptable if the older legacy key won here")
	}
	if after := mustRead(t, legacyPathIn(cwd)); !bytes.Equal(after, legacyKey) {
		t.Error("the ignored legacy key was modified")
	}
}

// The same tie-break for the user-config-dir form of the default location, so
// the rule is about "the current default holds a key", not about storage.
func TestResolveKeyPath_UserConfigKeyWinsOverLegacyKey(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)

	writeKeyFile(t, legacyPathIn(cwd), 29)
	configKey := filepath.Join(home, "openlocalcrm", defaultKeyFileName)
	derivedKey := writeKeyFile(t, configKey, 31)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("resolveKeyPath: %v", err)
	}
	if res.Source != KeyPathUserConfig || res.Path != configKey {
		t.Fatalf("expected the user-config key to win, got %+v", res)
	}
	if res.IgnoredLegacyPath != legacyPathIn(cwd) {
		t.Errorf("expected the legacy file to be reported, got %q", res.IgnoredLegacyPath)
	}

	t.Chdir(cwd)
	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(key, derivedKey) {
		t.Fatal("expected the user-config key to be loaded")
	}
}

// (d) With no legacy file anywhere the chain behaves exactly as before the
// rescue step was added: derive, mint, or fail - never a working-directory
// default.
func TestResolveKeyPath_NoLegacyKeyAnywhereUnchanged(t *testing.T) {
	clearKeyEnv(t)

	emptyCwd := t.TempDir()
	storage := t.TempDir()
	home := t.TempDir()

	t.Run("storage", func(t *testing.T) {
		t.Setenv("STORAGE_PATH", storage)
		res, err := resolveKeyPath(emptyCwd)
		if err != nil {
			t.Fatalf("resolveKeyPath: %v", err)
		}
		want := derivedPathIn(storage)
		if res.Path != want || res.CreatePath != want || res.Source != KeyPathStorage || res.IgnoredLegacyPath != "" {
			t.Fatalf("unexpected resolution %+v (want %q)", res, want)
		}
		if res.Notice() != "" {
			t.Errorf("nothing to warn about, got %q", res.Notice())
		}
	})

	t.Run("user config dir", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", home)
		t.Setenv("APPDATA", home)
		t.Setenv("HOME", home)
		res, err := resolveKeyPath(emptyCwd)
		if err != nil {
			t.Fatalf("resolveKeyPath: %v", err)
		}
		want := filepath.Join(home, "openlocalcrm", defaultKeyFileName)
		if res.Path != want || res.CreatePath != want || res.Source != KeyPathUserConfig {
			t.Fatalf("unexpected resolution %+v (want %q)", res, want)
		}
	})

	t.Run("unresolvable", func(t *testing.T) {
		res, err := resolveKeyPath(emptyCwd)
		if err == nil {
			t.Fatalf("expected the old error, got %+v", res)
		}
		if !strings.Contains(err.Error(), "SECRETS_KEY_PATH") {
			t.Errorf("error should name the variable to set, got: %v", err)
		}
		if res.Path != "" || res.CreatePath != "" {
			t.Errorf("expected an empty resolution on error, got %+v", res)
		}
	})
}

// (e) The #122 guarantee, restated with the rescue step in place: minting never
// writes into the working directory - not even a "data" directory.
func TestGetOrCreateMasterKey_MintDoesNotCreateLegacyLocation(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)

	key, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected a 32-byte key, got %d bytes", len(key))
	}
	if entries, err := os.ReadDir(cwd); err != nil {
		t.Fatalf("readdir %s: %v", cwd, err)
	} else if len(entries) != 0 {
		t.Errorf("the working directory was written into: %v", entries)
	}
	if regularFileExists(legacyPathIn(cwd)) {
		t.Errorf("a key was created at the legacy location %s (issue #122)", legacyPathIn(cwd))
	}
	if !bytes.Equal(mustRead(t, filepath.Join(home, "openlocalcrm", defaultKeyFileName)), key) {
		t.Error("the minted key is not the one at the resolved location")
	}
}

// A legacy file that exists but cannot be used is a hard error: the rescue must
// never turn into a write into the working directory.
func TestGetOrCreateMasterKey_NeverRepairsLegacyLocation(t *testing.T) {
	tests := []struct {
		name string
		// prepare makes the legacy file unusable and returns the content that
		// must still be on disk afterwards, or nil when the file cannot be
		// read at all (permission denied).
		prepare  func(t *testing.T, path string) []byte
		contains string
	}{
		{
			name: "corrupt content",
			prepare: func(t *testing.T, path string) []byte {
				content := []byte("not a master key")
				if err := os.WriteFile(path, content, 0600); err != nil {
					t.Fatalf("write: %v", err)
				}
				return content
			},
			contains: "refusing to overwrite",
		},
		{
			name: "unreadable",
			prepare: func(t *testing.T, path string) []byte {
				if os.Geteuid() == 0 {
					t.Skip("running as root: file permissions are not enforced")
				}
				if err := os.Chmod(path, 0000); err != nil {
					t.Fatalf("chmod: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
				return nil
			},
			contains: "cannot read the legacy master key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearKeyEnv(t)

			cwd := t.TempDir()
			storage := t.TempDir()
			if err := os.MkdirAll(filepath.Dir(legacyPathIn(cwd)), 0700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			// A file that is neither 32 raw bytes nor base64 of 32 bytes.
			if err := os.WriteFile(legacyPathIn(cwd), []byte("placeholder"), 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			wantContent := tc.prepare(t, legacyPathIn(cwd))
			t.Setenv("STORAGE_PATH", storage)
			t.Chdir(cwd)

			_, err := GetOrCreateMasterKey()
			if err == nil {
				t.Fatal("expected an error instead of repairing the legacy key")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("expected the error to contain %q, got: %v", tc.contains, err)
			}
			if !strings.Contains(err.Error(), legacyPathIn(cwd)) {
				t.Errorf("the error must name the file it refuses to touch, got: %v", err)
			}
			if !strings.Contains(err.Error(), "SECRETS_KEY_PATH") {
				t.Errorf("the error should name the pin, got: %v", err)
			}
			// Nothing was created, in the working directory or anywhere else.
			if regularFileExists(derivedPathIn(storage)) {
				t.Errorf("a replacement key was minted at %s while the legacy file is broken", derivedPathIn(storage))
			}
			if wantContent != nil {
				if after := mustRead(t, legacyPathIn(cwd)); !bytes.Equal(after, wantContent) {
					t.Errorf("the legacy file was rewritten: got %q, want %q", after, wantContent)
				}
			}
		})
	}
}

// The warning is emitted once per process, on the first resolution that needs
// it, and is loud enough to find in the log.
func TestLogKeyNoticeOncePerProcess(t *testing.T) {
	clearKeyEnv(t)

	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	res := KeyResolution{
		Path:       legacyPathIn("/srv/crm"),
		Source:     KeyPathLegacy,
		CreatePath: "/data/storage/keys/secrets.key",
	}
	notice := res.Notice()
	logKeyNotice(notice)
	logKeyNotice(notice)

	out := buf.String()
	if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != 1 {
		t.Errorf("expected the warning to be logged once, got %d lines:\n%s", lines, out)
	}
	if !strings.Contains(out, "[Crypto] [WARN]") {
		t.Errorf("expected the [Crypto] [WARN] prefix, got %q", out)
	}
	if !strings.Contains(out, legacyPathIn("/srv/crm")) || !strings.Contains(out, "/data/storage/keys/secrets.key") {
		t.Errorf("the warning must name the legacy and the recommended location, got %q", out)
	}

	// GetOrCreateMasterKey routes the real resolution through it once.
	resetKeyNoticeForTest()
	buf.Reset()
	cwd := t.TempDir()
	writeKeyFile(t, legacyPathIn(cwd), 41)
	t.Chdir(cwd)
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)
	if _, err := GetOrCreateMasterKey(); err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if _, err := GetOrCreateMasterKey(); err != nil {
		t.Fatalf("GetOrCreateMasterKey (second call): %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "reusing the existing legacy master key") {
		t.Errorf("expected the legacy-reuse warning from GetOrCreateMasterKey, got %q", got)
	}
	if strings.Count(got, "[Crypto] [WARN]") != 1 {
		t.Errorf("expected exactly one warning, got %q", got)
	}
}

// The legacy candidate set is explicit: the cwd-relative old default first,
// then the documented container path, deduplicated.
func TestLegacyKeyCandidates(t *testing.T) {
	clearKeyEnv(t)

	// The list itself is pure, so it can be checked against the production
	// container path without touching the filesystem.
	t.Run("list", func(t *testing.T) {
		useRealContainerCandidate(t)

		if got := legacyKeyCandidates("/srv/crm"); len(got) != 2 || got[0] != "/srv/crm/data/secrets.key" || got[1] != "/app/data/secrets.key" {
			t.Errorf("unexpected candidate list: %v", got)
		}
		// The container case: cwd is /app, so the two entries are the same file.
		if got := legacyKeyCandidates("/app"); len(got) != 1 || got[0] != "/app/data/secrets.key" {
			t.Errorf("expected the duplicate to collapse, got %v", got)
		}
		if got := legacyKeyCandidates(""); len(got) != 1 || got[0] != "/app/data/secrets.key" {
			t.Errorf("an unreadable cwd must still leave the container candidate, got %v", got)
		}
	})

	// Probing stays on the sandboxed container path (which never exists), so
	// this is deterministic even inside a container built from a pre-#122 image.
	t.Run("the derived location is never reported as legacy", func(t *testing.T) {
		cwd := t.TempDir()
		derived := filepath.Join(cwd, "data", "keys", defaultKeyFileName)
		if got := firstExistingLegacyKeyPath(cwd, derived); got != "" {
			t.Errorf("expected no legacy candidate for %q, got %q", cwd, got)
		}
		writeKeyFile(t, legacyPathIn(cwd), 43)
		if got := firstExistingLegacyKeyPath(cwd, legacyPathIn(cwd)); got != "" {
			t.Errorf("the derived location must be skipped, got %q", got)
		}
		if got := firstExistingLegacyKeyPath(cwd, derived); got != legacyPathIn(cwd) {
			t.Errorf("expected the existing legacy file, got %q", got)
		}
	})
}

// A directory is not a key file: resolution must not stop at a stray data/
// directory in the working tree.
func TestResolveKeyPath_IgnoresNonRegularLegacyEntries(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	if err := os.MkdirAll(legacyPathIn(cwd), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("resolveKeyPath: %v", err)
	}
	if res.Source != KeyPathStorage || res.Path != derivedPathIn(storage) {
		t.Fatalf("a directory is not an existing key: %+v", res)
	}
	if res.IgnoredLegacyPath != "" {
		t.Errorf("expected nothing ignored, got %q", res.IgnoredLegacyPath)
	}
}

// Legacy rescue when nothing else can be derived at all (no STORAGE_PATH, no
// usable user config dir): the legacy file is still reused, the warning says
// there is no replacement location, and the process does not fail.
func TestResolveKeyPath_LegacyRescueWithoutDerivableLocation(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	key := writeKeyFile(t, legacyPathIn(cwd), 57)

	res, err := resolveKeyPath(cwd)
	if err != nil {
		t.Fatalf("a readable legacy key must be enough: %v", err)
	}
	if res.Source != KeyPathLegacy || res.Path != legacyPathIn(cwd) {
		t.Fatalf("unexpected resolution %+v", res)
	}
	if res.CreatePath != "" {
		t.Errorf("no location is derivable, CreatePath must stay empty, got %q", res.CreatePath)
	}
	notice := res.Notice()
	if !strings.Contains(notice, "no replacement location could be derived") || !strings.Contains(notice, "SECRETS_KEY_PATH") {
		t.Errorf("the warning must say that nothing could be derived and name the pin, got %q", notice)
	}

	got, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Fatal("expected the legacy key bytes")
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the legacy data/ directory, got %v", entries)
	}
}

// Small helpers with security-relevant edge inputs: an empty path is never a
// key file, and path comparison is cleaning-normalised.
func TestKeyPathHelpers(t *testing.T) {
	if regularFileExists("") {
		t.Error("the empty path must not count as an existing key")
	}
	if regularFileExists(t.TempDir()) {
		t.Error("a directory is not a regular file")
	}
	if samePath("", "/a/b") || samePath("/a/b", "") {
		t.Error("an empty path never equals another")
	}
	if !samePath("/a/b", "/a/./b") {
		t.Error("/a/b and /a/./b are the same location")
	}
}

// The base64 key shape is documented and load-bearing: a stored credential
// encrypted with such a key must keep decrypting after a restart.
func TestGetOrCreateMasterKey_LoadsBase64EncodedKey(t *testing.T) {
	clearKeyEnv(t)

	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(200 + i)
	}
	path := filepath.Join(t.TempDir(), defaultKeyFileName)
	encoded := base64.StdEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("SECRETS_KEY_PATH", path)

	got, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("expected the decoded key %x, got %x", raw, got)
	}
	if after := mustRead(t, path); string(after) != encoded {
		t.Error("a base64 key file was rewritten instead of loaded")
	}
}

// SetMasterKeyForTest with a real key is what every other package uses; it must
// win over the whole resolution chain (no file access, no error even when
// nothing is configurable).
func TestSetMasterKeyForTest_ShortCircuitsResolution(t *testing.T) {
	clearKeyEnv(t)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i * 3)
	}
	SetMasterKeyForTest(key)

	got, err := GetOrCreateMasterKey()
	if err != nil {
		t.Fatalf("GetOrCreateMasterKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Errorf("expected the injected key, got %x", got)
	}
	sealed, err := EncryptSecret("nur-getestet")
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	opened, err := DecryptSecret(sealed)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if opened != "nur-getestet" {
		t.Errorf("roundtrip through the injected key failed: %q", opened)
	}
}

// Concurrent first use is the real server/worker shape (SSE handlers, jobs).
// It exercises the double-checked cache path, the legacy notice latch and the
// new mutex pairing without deadlocking, and every caller must get one key.
func TestGetOrCreateMasterKey_ConcurrentFirstUse(t *testing.T) {
	clearKeyEnv(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	legacyKey := writeKeyFile(t, legacyPathIn(cwd), 61)
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	var wg sync.WaitGroup
	results := make([][]byte, 8)
	errs := make([]error, len(results))
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key, err := GetOrCreateMasterKey()
			results[i], errs[i] = key, err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if !bytes.Equal(results[i], legacyKey) {
			t.Fatalf("goroutine %d got a different key - secrets would split across two masters", i)
		}
	}
	if regularFileExists(derivedPathIn(storage)) {
		t.Error("a racing goroutine minted a second key in the storage volume")
	}
}
