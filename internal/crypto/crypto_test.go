package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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
