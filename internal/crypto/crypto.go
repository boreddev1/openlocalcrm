package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	keyCache []byte
	keyMu    sync.RWMutex
)

// defaultKeyFileName is the file name used when the key location is derived
// from another directory instead of being configured outright.
const defaultKeyFileName = "secrets.key"

// GetKeyPath returns the resolved file path for the master secret key.
//
// Resolution order - the first entry that yields a value wins:
//
//  1. SECRETS_KEY_PATH (canonical)
//  2. SECRET_KEY_PATH (legacy alias, kept for existing installs)
//  3. $STORAGE_PATH/keys/secrets.key, or $STORAGE_LOCAL_DIR/... (the same
//     alias chain cmd/server uses for the storage root). docker-compose.yml,
//     docker-compose.demo.yml and the launcher template all set STORAGE_PATH
//     to a directory that is mounted from a persisted volume, so server and
//     worker read one and the same key with no extra configuration.
//  4. <user config dir>/openlocalcrm/secrets.key (os.UserConfigDir:
//     $XDG_CONFIG_HOME, %AppData%, ~/Library/Application Support) - the
//     locally-run and launcher-installed case where no storage root is set.
//
// There is deliberately no working-directory default. The old fallback was
// "data/secrets.key" relative to the process cwd, so every
// `go test ./internal/auth/...` generated a fresh 256-bit master key straight
// into the package directory (issue #122). Only the `*.key` filename rule in
// .gitignore hid it; any sibling file in that directory stays committable, so
// a `git add -A` after a test run could ship a real key.
//
// An explicitly configured path is honored verbatim - the operator owns it.
// If nothing can be resolved the error is returned instead of silently
// picking a location: a wrong or unwritable key path must fail loudly, the
// same way cmd/server treats an unpersistable JWT signing key.
func GetKeyPath() (string, error) {
	if path := firstConfiguredEnv("SECRETS_KEY_PATH", "SECRET_KEY_PATH"); path != "" {
		return path, nil
	}
	if storage := firstConfiguredEnv("STORAGE_PATH", "STORAGE_LOCAL_DIR"); storage != "" {
		return filepath.Join(storage, "keys", defaultKeyFileName), nil
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "openlocalcrm", defaultKeyFileName), nil
	}
	return "", errors.New("cannot resolve a location for the master secret key: " +
		"set SECRETS_KEY_PATH to an absolute path (e.g. /app/keys/secrets.key), " +
		"or STORAGE_PATH so it can be derived from the storage directory")
}

// firstConfiguredEnv returns the first non-empty value among names.
func firstConfiguredEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// GetOrCreateMasterKey loads the master 256-bit AES key from the path resolved
// by GetKeyPath, or generates a new one with 0600 permissions if absent.
func GetOrCreateMasterKey() ([]byte, error) {
	keyMu.RLock()
	if len(keyCache) == 32 {
		defer keyMu.RUnlock()
		cp := make([]byte, 32)
		copy(cp, keyCache)
		return cp, nil
	}
	keyMu.RUnlock()

	keyMu.Lock()
	defer keyMu.Unlock()

	if len(keyCache) == 32 {
		cp := make([]byte, 32)
		copy(cp, keyCache)
		return cp, nil
	}

	keyPath, err := GetKeyPath()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) == 32 {
			keyCache = make([]byte, 32)
			copy(keyCache, data)
			return data, nil
		}
		// If key is base64 encoded
		if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil && len(decoded) == 32 {
			keyCache = make([]byte, 32)
			copy(keyCache, decoded)
			return decoded, nil
		}
	}

	// Generate new 32-byte key
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("failed to generate random secret key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory for secret key: %w", err)
	}

	if err := os.WriteFile(keyPath, newKey, 0600); err != nil {
		return nil, fmt.Errorf("failed to write secret key to %s: %w", keyPath, err)
	}

	keyCache = make([]byte, 32)
	copy(keyCache, newKey)
	return newKey, nil
}

// SetMasterKeyForTest sets the cached key for testing purposes.
func SetMasterKeyForTest(key []byte) {
	keyMu.Lock()
	defer keyMu.Unlock()
	if key == nil {
		keyCache = nil
		return
	}
	keyCache = make([]byte, len(key))
	copy(keyCache, key)
}

// Encrypt encrypts plaintext using AES-256-GCM and returns standard base64 string.
// Output format: base64( 12-byte-nonce + ciphertext + 16-byte-tag )
func Encrypt(plaintext []byte) (string, error) {
	key, err := GetOrCreateMasterKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
func Decrypt(encodedCiphertext string) ([]byte, error) {
	if encodedCiphertext == "" {
		return nil, errors.New("empty ciphertext")
	}

	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 ciphertext: %w", err)
	}

	key, err := GetOrCreateMasterKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptSecret encrypts a plaintext secret string using AES-256-GCM.
func EncryptSecret(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	return Encrypt([]byte(secret))
}

// DecryptSecret decrypts a base64 AES-256-GCM encrypted secret string.
func DecryptSecret(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	dec, err := Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}
