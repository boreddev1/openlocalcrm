package storage

import (
	"context"
	"errors"
	"io"
)

var (
	ErrFileNotFound = errors.New("file not found")
	ErrInvalidPath  = errors.New("invalid storage path")
)

// StorageService defines an interchangeable interface for file operations
type StorageService interface {
	Save(ctx context.Context, filename string, r io.Reader) (string, error)
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (int64, error)
}
