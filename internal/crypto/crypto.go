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

// GetKeyPath returns the resolved file path for the master secret key.
func GetKeyPath() string {
	path := os.Getenv("SECRETS_KEY_PATH")
	if path == "" {
		path = os.Getenv("SECRET_KEY_PATH")
	}
	if path == "" {
		path = "data/secrets.key"
	}
	return path
}

// GetOrCreateMasterKey loads the master 256-bit AES key from SECRETS_KEY_PATH,
// or generates a new one with 0600 permissions if absent.
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

	keyPath := GetKeyPath()
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
