package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// ErrKeyUnusable classifies the fail-closed master key errors (issue #128):
// a key file is present (or its state could not be verified) but cannot be
// used, so the process refuses to mint a replacement. Callers can detect the
// class with errors.Is without echoing the detailed cause - which names
// filesystem paths and recovery steps - to end users.
var ErrKeyUnusable = errors.New("master key unusable")

var (
	keyCache []byte
	keyMu    sync.RWMutex

	// keyNoticeMu guards the once-per-process latch for the key-location
	// warning. It is deliberately separate from keyMu: logKeyNotice is called
	// while GetOrCreateMasterKey holds keyMu.
	keyNoticeMu     sync.Mutex
	keyNoticeLogged bool
)

// defaultKeyFileName is the file name used when the key location is derived
// from another directory instead of being configured outright.
const defaultKeyFileName = "secrets.key"

// legacyKeyRelPath is the pre-#122 default location, relative to the process
// working directory - the old code literally did `path = "data/secrets.key"`.
var legacyKeyRelPath = filepath.Join("data", defaultKeyFileName)

// legacyKeyContainerPath is that same file for the documented container
// installs: Dockerfile.server:35 and Dockerfile.worker:13 both set
// WORKDIR /app, so their cwd-relative default was /app/data/secrets.key - an
// image-layer path with no volume behind it (R-05,
// docs/review/2026-09-30/03-security-auth.md). It is listed separately so the
// read-only rescue still finds the key if someone runs the image with a
// different working directory. A var (not a const) only so the unit tests can
// sandbox it - see clearKeyEnv in crypto_test.go.
var legacyKeyContainerPath = "/app/data/secrets.key"

// KeyPathSource names which step of the resolution chain produced a path.
type KeyPathSource string

const (
	// KeyPathExplicit: SECRETS_KEY_PATH / SECRET_KEY_PATH, honored verbatim.
	KeyPathExplicit KeyPathSource = "explicit"
	// KeyPathStorage: derived from STORAGE_PATH / STORAGE_LOCAL_DIR.
	KeyPathStorage KeyPathSource = "storage"
	// KeyPathUserConfig: derived from os.UserConfigDir().
	KeyPathUserConfig KeyPathSource = "userconfig"
	// KeyPathLegacy: an existing pre-#122 key reused read-only. A legacy
	// location is never created, repaired or overwritten.
	KeyPathLegacy KeyPathSource = "legacy"
)

// KeyResolution is the full answer of ResolveKeyPath: which file to read, why
// that one, where a brand-new key may be written, and whether an unused legacy
// key is sitting around that the operator should know about.
type KeyResolution struct {
	// Path is the file the master key is read from.
	Path string
	// Source is the resolution step that produced Path.
	Source KeyPathSource
	// CreatePath is where GetOrCreateMasterKey is allowed to write a new key.
	// It differs from Path only for a legacy rescue (where it stays the normal
	// derived location, which is precisely why nothing is written there), and
	// is empty when no location can be derived at all. It is never a legacy
	// location.
	CreatePath string
	// IgnoredLegacyPath is a legacy key file that exists but was not chosen,
	// because a key already lives at the current default location.
	IgnoredLegacyPath string
}

// Notice returns the operator-facing warning for this resolution, or "" when
// there is nothing to report. Pure, so tests can assert the text without
// capturing log output.
func (r KeyResolution) Notice() string {
	switch r.Source {
	case KeyPathLegacy:
		if r.CreatePath == "" {
			return fmt.Sprintf("reusing the existing legacy master key at %s; that location is "+
				"deprecated and read-only (the server never creates or repairs a key there) and no "+
				"replacement location could be derived - set SECRETS_KEY_PATH", r.Path)
		}
		return fmt.Sprintf("reusing the existing legacy master key at %s; that location is deprecated "+
			"and read-only (the server never creates or repairs a key there). To silence this warning, "+
			"copy the file to %s or pin SECRETS_KEY_PATH to the key that should be authoritative",
			r.Path, r.CreatePath)
	case KeyPathStorage, KeyPathUserConfig:
		if r.IgnoredLegacyPath != "" {
			return fmt.Sprintf("using master key %s while an unused legacy key exists at %s; if stored "+
				"secrets stop decrypting after an upgrade, that legacy file is the one to copy to %s or "+
				"pin with SECRETS_KEY_PATH", r.Path, r.IgnoredLegacyPath, r.Path)
		}
	}
	return ""
}

// GetKeyPath returns the resolved file path for the master secret key. It is
// the path-only form of ResolveKeyPath; see that function for the order and
// the tie-break rules.
func GetKeyPath() (string, error) {
	res, err := ResolveKeyPath()
	if err != nil {
		return "", err
	}
	return res.Path, nil
}

// ResolveKeyPath resolves the location of the master secret key.
//
// Resolution order - the first entry that yields a value wins:
//
//  1. SECRETS_KEY_PATH (canonical) - honored verbatim, the operator owns it.
//  2. SECRET_KEY_PATH (legacy environment alias) - same.
//  3. The current default location, but only if a key file already exists
//     there: $STORAGE_PATH/keys/secrets.key, or $STORAGE_LOCAL_DIR/... (the
//     same alias chain cmd/server uses for the storage root).
//     docker-compose.yml, docker-compose.demo.yml and the launcher template
//     all set STORAGE_PATH to a directory mounted from a persisted volume, so
//     server and worker read one and the same key with no extra configuration.
//     With no storage root configured:
//     <user config dir>/openlocalcrm/secrets.key (os.UserConfigDir:
//     $XDG_CONFIG_HOME, %AppData%, ~/Library/Application Support) - the
//     locally-run and launcher-installed case.
//  4. An EXISTING key file at a pre-#122 legacy location: <cwd>/data/secrets.key,
//     then /app/data/secrets.key. This step is a rescue, not a default: it can
//     only point at a file that is already on disk, it is never a write
//     target, and choosing it logs a one-time warning (KeyResolution.Notice).
//  5. Otherwise the location from step 3 is returned as the place where a new
//     key will be minted. If nothing exists anywhere and no location can be
//     derived, an error is returned instead of inventing a path: a wrong or
//     unwritable key path must fail loudly, the same way cmd/server treats an
//     unpersistable JWT signing key.
//
// Tie-break between step 3 and step 4 - documented, because two keys on disk
// means exactly one of them can be authoritative: if BOTH a legacy file and a
// file at the current default location exist, the current default location
// wins and the legacy file is reported through IgnoredLegacyPath. Reasoning:
// the default location is only populated either by the operator copying the
// legacy key there (identical bytes - the choice is then a no-op) or by a
// process that already resolved with this order and encrypted newer secrets
// with it. Reaching back for the legacy file in that situation would silently
// invalidate the newer data, which is the very failure class #122 removed. A
// legacy file is therefore used only while the default location still has no
// key - i.e. on the first start of an upgraded install, which is the case that
// would otherwise lose every stored TOTP and email credential.
//
// A relative SECRETS_KEY_PATH / SECRET_KEY_PATH / STORAGE_PATH is honored as
// configured - the operator owns that location and it may well be inside the
// working directory. The "never write into the working directory" guarantee
// covers only the places this package picks for itself (steps 3-5), which is
// what made `go test` a key generator in the first place.
//
// There is deliberately no working-directory *default*. The old fallback was
// "data/secrets.key" relative to the process cwd, so every
// `go test ./internal/auth/...` generated a fresh 256-bit master key straight
// into the package directory (issue #122). Only the `*.key` filename rule in
// .gitignore hid it; any sibling file in that directory stays committable, so
// a `git add -A` after a test run could ship a real key. Step 4 may read such
// a file if an older build already wrote one; no code path in this package
// creates a key at a legacy location.
func ResolveKeyPath() (KeyResolution, error) {
	// Best effort: an unreadable cwd only drops the cwd-relative legacy
	// candidate, it must not break resolution for an install that never had
	// one to begin with.
	cwd, _ := os.Getwd()
	return resolveKeyPath(cwd)
}

func resolveKeyPath(cwd string) (KeyResolution, error) {
	if path := firstConfiguredEnv("SECRETS_KEY_PATH", "SECRET_KEY_PATH"); path != "" {
		return KeyResolution{Path: path, Source: KeyPathExplicit, CreatePath: path}, nil
	}

	derived, derivedSource, derivedErr := derivedKeyPath()
	derivedExists := derivedErr == nil && regularFileExists(derived)
	legacy := firstExistingLegacyKeyPath(cwd, derived)

	switch {
	case derivedExists:
		// The current default location holds a key: it is authoritative and
		// any legacy file is only reported, never used.
		return KeyResolution{Path: derived, Source: derivedSource, CreatePath: derived, IgnoredLegacyPath: legacy}, nil
	case legacy != "":
		// Read-only rescue of a pre-#122 install: keep decrypting with the key
		// it already has; a brand-new key would still land on the default.
		return KeyResolution{Path: legacy, Source: KeyPathLegacy, CreatePath: derived}, nil
	case derivedErr != nil:
		return KeyResolution{}, derivedErr
	default:
		return KeyResolution{Path: derived, Source: derivedSource, CreatePath: derived}, nil
	}
}

// derivedKeyPath computes the current default location (steps 3 and 5) and
// which rule produced it.
func derivedKeyPath() (string, KeyPathSource, error) {
	if storage := firstConfiguredEnv("STORAGE_PATH", "STORAGE_LOCAL_DIR"); storage != "" {
		return filepath.Join(storage, "keys", defaultKeyFileName), KeyPathStorage, nil
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "openlocalcrm", defaultKeyFileName), KeyPathUserConfig, nil
	}
	return "", "", errors.New("cannot resolve a location for the master secret key: " +
		"set SECRETS_KEY_PATH to an absolute path (e.g. /app/keys/secrets.key), " +
		"or STORAGE_PATH so it can be derived from the storage directory")
}

// legacyKeyCandidates lists the pre-#122 locations in precedence order. Only
// files that already exist are ever considered; nothing here is created.
func legacyKeyCandidates(cwd string) []string {
	candidates := make([]string, 0, 2)
	add := func(path string) {
		for _, have := range candidates {
			if samePath(have, path) {
				return
			}
		}
		candidates = append(candidates, path)
	}
	if cwd != "" {
		add(filepath.Join(cwd, legacyKeyRelPath))
	}
	add(legacyKeyContainerPath)
	return candidates
}

// firstExistingLegacyKeyPath returns the first legacy key file that exists,
// skipping anything that is the derived location itself.
func firstExistingLegacyKeyPath(cwd, derived string) string {
	for _, candidate := range legacyKeyCandidates(cwd) {
		if samePath(candidate, derived) {
			continue
		}
		if regularFileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// regularFileExists reports whether path names an existing regular file.
func regularFileExists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// logKeyNotice prints an operator warning at most once per process - the
// resolution itself is pure and may be queried repeatedly.
func logKeyNotice(msg string) {
	if msg == "" {
		return
	}
	keyNoticeMu.Lock()
	defer keyNoticeMu.Unlock()
	if keyNoticeLogged {
		return
	}
	keyNoticeLogged = true
	//nolint:gosec // G706: the value is operator configuration (a path), not attacker-controlled text
	log.Printf("[Crypto] [WARN] %s", msg)
}

// resetKeyNoticeForTest clears the once-per-process latch.
func resetKeyNoticeForTest() {
	keyNoticeMu.Lock()
	defer keyNoticeMu.Unlock()
	keyNoticeLogged = false
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

// GetOrCreateMasterKey loads the master 256-bit AES key from the location
// resolved by ResolveKeyPath, or generates a new one with 0600 permissions at
// KeyResolution.CreatePath if no key file exists there yet.
//
// Fail-closed rule (issue #128): "no key yet" and "key present but unusable"
// are different states. A file that exists but cannot be read, or that cannot
// be parsed as a key - empty, truncated, wrong byte length, garbage - is a
// hard error that names the path and the cause; it is never repaired by
// silently minting a replacement. Regenerating over a present key would keep
// the process looking healthy while making every secret already encrypted
// with the old key (stored mail account passwords, TOTP seeds) permanently
// undecryptable - the same data-loss class that cmd/server handles by
// refusing to start without a usable JWT key and PR #84 fixed for the CSRF
// secret ("persist instead of regenerating").
//
// An existing file is never truncated or overwritten, not even an invalid
// one: minting writes a 0600 temp file in the target directory and publishes
// it with an atomic rename, re-checking that no file appeared before the
// rename. After publishing, the bytes on disk - not the bytes this process
// generated - become the cached key, so a server and worker starting
// concurrently for the first time converge on one key instead of each
// keeping a different one in memory.
//
// A key resolved from a legacy location is only ever read: if it cannot be
// read or parsed, this returns an error rather than writing into (or
// repairing) that deprecated place (#127).
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

	res, err := ResolveKeyPath()
	if err != nil {
		return nil, err
	}
	logKeyNotice(res.Notice())

	if res.Source == KeyPathLegacy {
		// The location exists (that is why it was chosen) but is deprecated.
		// Anything other than a usable key there is a hard error: minting a
		// replacement would be the silent data loss this whole path avoids,
		// and writing it would put a key back into the working directory.
		data, readErr := os.ReadFile(res.Path)
		if readErr != nil {
			return nil, fmt.Errorf("%w: cannot read the legacy master key at %s: %v - refusing to create a "+
				"replacement there; copy the file to %s or set SECRETS_KEY_PATH",
				ErrKeyUnusable, res.Path, readErr, res.CreatePath)
		}
		key, ok := parseStoredKey(data)
		if !ok {
			return nil, fmt.Errorf("%w: legacy master key at %s is neither a raw 32-byte key nor a "+
				"base64-encoded one (%s) - refusing to overwrite it; copy a valid key to %s or set SECRETS_KEY_PATH",
				ErrKeyUnusable, res.Path, describeKeyBytes(data), res.CreatePath)
		}
		return cacheMasterKeyLocked(key), nil
	}

	data, readErr := os.ReadFile(res.Path)
	if readErr == nil {
		key, ok := parseStoredKey(data)
		if !ok {
			return nil, unusableMasterKeyError(res.Path, data, res.IgnoredLegacyPath)
		}
		return cacheMasterKeyLocked(key), nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		// The state of the file could not be verified at all (permissions,
		// a path component that is not a directory, ...): it may exist and it
		// may already protect stored data, so this is "present but unusable",
		// not "absent".
		return nil, unreadableMasterKeyError(res.Path, readErr)
	}

	// Genuinely no key file yet: mint one and publish it at the writable
	// location.
	if res.CreatePath == "" {
		return nil, errors.New("cannot create a master key: no writable key location is configured " +
			"(set SECRETS_KEY_PATH, or STORAGE_PATH so it can be derived from the storage directory)")
	}
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("failed to generate random secret key: %w", err)
	}
	if _, err := publishNewKeyFile(res.CreatePath, newKey); err != nil {
		return nil, err
	}

	// Adopt what is on disk rather than what this process generated: if a
	// concurrent first start (server and worker boot together) published its
	// key between the existence check and the rename, both converge on the
	// file, and a key nobody ever encrypted with is the one that gets dropped.
	data, readErr = os.ReadFile(res.CreatePath)
	if readErr != nil {
		return nil, unreadableMasterKeyError(res.CreatePath, readErr)
	}
	key, ok := parseStoredKey(data)
	if !ok {
		return nil, unusableMasterKeyError(res.CreatePath, data, "")
	}
	return cacheMasterKeyLocked(key), nil
}

// cacheMasterKeyLocked stores key in the process cache and returns a copy.
// Call only with keyMu held.
func cacheMasterKeyLocked(key []byte) []byte {
	keyCache = make([]byte, 32)
	copy(keyCache, key)
	return append([]byte(nil), key...)
}

// publishNewKeyFile writes a freshly minted key at path and reports whether
// this process's file is the one on disk. It refuses to touch an existing
// file - valid, invalid, whatever: if a key file is present before the
// rename, or appears between the check and the rename, nothing is written
// over it and the bool result is false so the caller adopts the file instead.
// The write itself goes to a 0600 temp file in the same directory plus an
// atomic rename, so a crash mid-write can never leave a truncated key where a
// healthy one used to be.
func publishNewKeyFile(path string, key []byte) (bool, error) {
	if regularFileExists(path) {
		return false, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil { //nolint:gosec // G301: 0700, and the path is operator configuration (issue #122)
		return false, fmt.Errorf("failed to create directory for secret key: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".new-*")
	if err != nil {
		return false, fmt.Errorf("failed to create temporary key file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(key); err != nil {
		cleanup()
		return false, fmt.Errorf("failed to write temporary key file %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0600); err != nil { //nolint:gosec // G302: owner-only, the master key must never be group/world readable
		cleanup()
		return false, fmt.Errorf("failed to set 0600 on temporary key file %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return false, fmt.Errorf("failed to flush temporary key file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return false, fmt.Errorf("failed to close temporary key file %s: %w", tmpName, err)
	}
	// Last look before publishing: an existing key file, even an invalid one,
	// is never replaced by a rename.
	if regularFileExists(path) {
		_ = os.Remove(tmpName)
		return false, nil
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return false, fmt.Errorf("failed to write secret key to %s: %w", path, err)
	}
	return true, nil
}

// unreadableMasterKeyError is the fail-closed error for a key location whose
// file could not be read. The cause is kept as part of the chain (errors.Is
// on both the underlying error and ErrKeyUnusable).
func unreadableMasterKeyError(path string, cause error) error {
	return fmt.Errorf("%w: cannot read the master key file at %s: %v; refusing to generate a replacement "+
		"because a key may already protect stored data (mail account passwords, TOTP seeds) at this path, and a "+
		"fresh key would silently orphan it. Make sure %s exists and is readable by the service user (check the "+
		"mode of the file and of its parent directories) or restore it from backup; to use another file, set "+
		"SECRETS_KEY_PATH", ErrKeyUnusable, path, cause, path)
}

// unusableMasterKeyError is the fail-closed error for a key file that is
// present but does not parse. Empty files, truncated files and wrong byte
// lengths are all "present but unusable" (issue #128), never "absent".
func unusableMasterKeyError(path string, data []byte, ignoredLegacy string) error {
	msg := fmt.Sprintf("the master key file at %s already exists but does not contain a usable key (%s). "+
		"Refusing to overwrite or regenerate it - every secret already encrypted with the previous key (stored "+
		"mail account passwords, TOTP seeds) would become permanently undecryptable while the server still looks "+
		"healthy. Recovery: restore the original content at %s, or point SECRETS_KEY_PATH at the correct key "+
		"file. Only if that key is definitively lost: delete %s and restart to mint a fresh key, then re-enter "+
		"every stored secret.", path, describeKeyBytes(data), path, path)
	if ignoredLegacy != "" {
		msg += fmt.Sprintf(" A pre-#122 legacy key file also exists at %s; if it holds the original key, copy it "+
			"to %s.", ignoredLegacy, path)
	}
	return fmt.Errorf("%w: %s", ErrKeyUnusable, msg)
}

// describeKeyBytes tells the operator what an unusable key file actually
// contains, so "empty", "truncated" and "wrong length" are distinguishable in
// the error without dumping key material into the log.
func describeKeyBytes(data []byte) string {
	switch {
	case len(data) == 0:
		return "empty file, 0 bytes"
	case len(data) < 32:
		return fmt.Sprintf("%d bytes, shorter than 32 - the file looks truncated", len(data))
	default:
		return fmt.Sprintf("%d bytes, expected 32 raw key bytes or their base64 encoding", len(data))
	}
}

// parseStoredKey accepts the two documented on-disk shapes of the master key:
// 32 raw bytes, or those bytes base64-encoded.
func parseStoredKey(data []byte) ([]byte, bool) {
	if len(data) == 32 {
		key := make([]byte, 32)
		copy(key, data)
		return key, true
	}
	if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil && len(decoded) == 32 {
		return decoded, true
	}
	return nil, false
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
