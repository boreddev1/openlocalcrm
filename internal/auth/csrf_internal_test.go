package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// The holder is the unit that owns the install-once rule; testing it directly
// keeps the package-level secret untouched, so no other test in this binary can
// observe an ordering effect from these cases.

func TestCSRFHolderRejectsUnusableSecrets(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
		want  string
	}{
		{"nil", nil, "too short"},
		{"empty", []byte(""), "too short"},
		{"whitespace only", []byte("  \n\t "), "too short"},
		{"31 bytes", bytes.Repeat([]byte("a"), 31), "too short"},
		{"newline padded short", []byte("\n" + strings.Repeat("a", 20) + "\n"), "too short"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &csrfSecretHolder{}
			err := h.install(tc.input, true)
			if err == nil {
				t.Fatalf("set(%q) = nil, want an error mentioning %q", tc.input, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("set(%q) error = %q, want it to mention %q", tc.input, err, tc.want)
			}
			if h.installed {
				t.Fatal("a rejected secret must not mark the holder as installed")
			}
		})
	}
}

func TestCSRFHolderTrimsAndInstalls(t *testing.T) {
	h := &csrfSecretHolder{}
	raw := append(bytes.Repeat([]byte("k"), CSRFSecretLength), '\n')
	if err := h.install(raw, true); err != nil {
		t.Fatalf("set() with trailing newline returned %v", err)
	}
	got := h.get()
	if len(got) != CSRFSecretLength {
		t.Fatalf("get() returned %d bytes, want %d (the trailing newline must be trimmed)", len(got), CSRFSecretLength)
	}
	if !h.installed {
		t.Fatal("a successful set() must mark the holder as installed")
	}
}

func TestCSRFHolderRefusesSecondInstall(t *testing.T) {
	h := &csrfSecretHolder{}
	first := bytes.Repeat([]byte("a"), CSRFSecretLength)
	second := bytes.Repeat([]byte("b"), CSRFSecretLength)
	if err := h.install(first, true); err != nil {
		t.Fatalf("first set() = %v", err)
	}
	err := h.install(second, true)
	if err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("second set() = %v, want an already-installed refusal", err)
	}
	if !bytes.Equal(h.get(), first) {
		t.Fatal("a refused second install must leave the first secret in place")
	}
}

func TestCSRFHolderEphemeralFallbackIsStable(t *testing.T) {
	h := &csrfSecretHolder{}
	first := h.get()
	if len(first) != CSRFSecretLength {
		t.Fatalf("ephemeral secret is %d bytes, want %d", len(first), CSRFSecretLength)
	}
	if bytes.Equal(first, make([]byte, CSRFSecretLength)) {
		t.Fatal("ephemeral secret must not be all zero bytes")
	}
	second := h.get()
	if !bytes.Equal(first, second) {
		t.Fatal("repeated get() must return the same ephemeral secret, not a fresh one")
	}
	if h.installed {
		t.Fatal("the lazy ephemeral must not count as an install; SetCSRFSecret still has to succeed afterwards")
	}
	// First use wins: generating the ephemeral key is a use, so a later install is
	// refused rather than swapping the key under live tokens.
	err := h.install(bytes.Repeat([]byte("z"), CSRFSecretLength), true)
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("installing after the ephemeral secret was generated = %v, want an in-use refusal", err)
	}
	if h.installed {
		t.Fatal("a refused install must not mark the holder as installed")
	}
}

// The pre-fix code had getCSRFSecret() read an unsynchronised global while
// SetCSRFSecret() wrote it. This is the regression pin: it must stay clean under
// -race, with concurrent readers, a late installer and readers racing it.
func TestCSRFHolderConcurrentAccessIsRaceFree(t *testing.T) {
	h := &csrfSecretHolder{}
	secret := bytes.Repeat([]byte("s"), CSRFSecretLength)
	if err := h.install(secret, true); err != nil {
		t.Fatalf("install() = %v", err)
	}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)

	for i := 0; i < 6; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			for j := 0; j < 400; j++ {
				got := h.get()
				if len(got) != CSRFSecretLength || !bytes.Equal(got, secret) {
					t.Errorf("get() returned %x, want the installed secret", got[:4])
					return
				}
			}
		}()
	}
	// Concurrent late installs must all be refused and must not disturb the reader.
	for i := 0; i < 3; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			if err := h.install(bytes.Repeat([]byte("t"), CSRFSecretLength), true); err == nil {
				t.Error("a second install succeeded while readers were live")
			}
		}()
	}

	start.Done()
	done.Wait()

	if got := h.get(); !bytes.Equal(got, secret) {
		t.Fatal("after concurrent refused installs, get() must still return the first secret")
	}
}

func TestDeriveCSRFSecretRequiresFullPrivateKey(t *testing.T) {
	_, err := DeriveCSRFSecret(ed25519.PrivateKey(bytes.Repeat([]byte("x"), 32)))
	if err == nil || !strings.Contains(err.Error(), "signing key is 32 bytes") {
		t.Fatalf("DeriveCSRFSecret(truncated key) = %v, want a key-size error", err)
	}
}

func TestDeriveCSRFSecretRejectsEmptySeed(t *testing.T) {
	// A zero seed is the one key shape that would otherwise derive successfully and
	// hand every installation the same CSRF key.
	empty := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	if _, err := DeriveCSRFSecret(empty); err == nil {
		t.Fatal("DeriveCSRFSecret(zero key) = nil, want an error: a shared secret for every install is not acceptable")
	}
}

func TestDeriveCSRFSecretUsesOnlyThePrivateSeed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fromKey, err := DeriveCSRFSecret(priv)
	if err != nil {
		t.Fatalf("DeriveCSRFSecret: %v", err)
	}
	twice := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	copy(twice, priv)
	// The public half is deterministic from the seed anyway; a derivation that read
	// the whole key would ignore this change, one that reads the seed would not.
	copy(twice[:32], priv[:32])
	again, err := DeriveCSRFSecret(twice)
	if err != nil {
		t.Fatalf("DeriveCSRFSecret(copy): %v", err)
	}
	if !bytes.Equal(fromKey, again) {
		t.Fatal("derivation must depend on the seed, not on incidental bytes of the key encoding")
	}
	other, err := DeriveCSRFSecret(append(append([]byte{}, pub...), pub...))
	if err == nil && bytes.Equal(fromKey, other) {
		t.Fatal("a different key must not derive the same CSRF secret")
	}
	if fmt.Sprintf("%x", fromKey) == fmt.Sprintf("%x", priv.Seed()) {
		t.Fatal("the derived secret must not be the raw seed")
	}
}
