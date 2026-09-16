package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage implements StorageService using the local filesystem volume
type LocalStorage struct {
	baseDir string
}

// NewLocalStorage creates a new LocalStorage instance rooted at baseDir
func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
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

	if _, err := io.Copy(tempFile, tee); err != nil {
		return "", fmt.Errorf("failed writing upload: %w", err)
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))[:16]
	cleanName := filepath.Base(filename)
	if cleanName == "" || cleanName == "." {
		cleanName = "attachment.bin"
	}
	finalName := fmt.Sprintf("%s_%s", hashStr, cleanName)
	finalPath := filepath.Join(targetDir, finalName)

	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return "", fmt.Errorf("failed committing uploaded file: %w", err)
	}

	return filepath.Join(relDir, finalName), nil
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
