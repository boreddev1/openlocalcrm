package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxUploadSize limits uploads to 50 MB.
const maxUploadSize = 50 * 1024 * 1024

// allowedMIMETypes are the only file types permitted in storage.
var allowedMIMETypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
	"text/plain":      true,
	"text/csv":        true,
}

// LocalStorage implements StorageService using the local filesystem volume
type LocalStorage struct {
	baseDir string
}

// NewLocalStorage creates a new LocalStorage instance rooted at baseDir
func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

// BaseDir returns the root directory for storage
func (s *LocalStorage) BaseDir() string {
	return s.baseDir
}

func (s *LocalStorage) sanitizeRelPath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func (s *LocalStorage) Save(ctx context.Context, filename string, r io.Reader) (string, error) {
	now := time.Now().UTC()
	relDir := filepath.Join(fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()))
	targetDir := filepath.Join(s.baseDir, relDir)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create storage dir: %w", err)
	}

	tempFile, err := os.CreateTemp(targetDir, "upload-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name()) // remove if not renamed
	}()

	hasher := sha256.New()
	tee := io.TeeReader(r, hasher)

	// Limit upload size to prevent abuse
	limited := io.LimitReader(tee, maxUploadSize+1)
	written, err := io.Copy(tempFile, limited)
	if err != nil {
		return "", fmt.Errorf("failed writing upload: %w", err)
	}
	if written > maxUploadSize {
		return "", fmt.Errorf("file exceeds maximum upload size of %d bytes", maxUploadSize)
	}

	// MIME-type validation via magic bytes MUST run before CAS dedup lookup to prevent cache poisoning
	if err := s.validateMIME(tempFile.Name()); err != nil {
		return "", err
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))[:16]
	cleanName := filepath.Base(filename)
	if cleanName == "" || cleanName == "." {
		cleanName = "attachment.bin"
	}
	finalName := fmt.Sprintf("%s_%s", hashStr, cleanName)
	finalPath := filepath.Join(targetDir, finalName)

	// CAS dedup: if a file with the same content hash already exists, return it
	if _, err := os.Stat(finalPath); err == nil {
		// File already exists with identical content hash – return the existing path
		return filepath.Join(relDir, finalName), nil
	}

	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return "", fmt.Errorf("failed committing uploaded file: %w", err)
	}

	return filepath.Join(relDir, finalName), nil
}

// validateMIME reads the first 512 bytes of the file to detect MIME type and enforces the allowlist.
func (s *LocalStorage) validateMIME(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file for MIME check: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file for MIME check: %w", err)
	}

	mimeType := http.DetectContentType(buf[:n])
	// Normalize: "text/plain; charset=utf-8" → "text/plain"
	if idx := bytes.IndexByte([]byte(mimeType), ';'); idx > 0 {
		mimeType = mimeType[:idx]
	}
	mimeType = strings.TrimSpace(mimeType)

	// Block HTML/SVG/Script tags disguised as text
	content := string(buf[:n])
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "<html") || strings.Contains(lowerContent, "<svg") || strings.Contains(lowerContent, "<script") {
		return fmt.Errorf("disallowed HTML/script content signature in uploaded file")
	}

	if !allowedMIMETypes[mimeType] {
		return fmt.Errorf("unsupported or disallowed file type: %s", mimeType)
	}

	return nil
}

func (s *LocalStorage) Open(ctx context.Context, relPath string) (io.ReadCloser, error) {
	cleanPath, err := s.sanitizeRelPath(relPath)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(s.baseDir, cleanPath)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, relPath string) error {
	cleanPath, err := s.sanitizeRelPath(relPath)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(s.baseDir, cleanPath)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) Stat(ctx context.Context, relPath string) (int64, error) {
	cleanPath, err := s.sanitizeRelPath(relPath)
	if err != nil {
		return 0, err
	}

	fullPath := filepath.Join(s.baseDir, cleanPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrFileNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}
