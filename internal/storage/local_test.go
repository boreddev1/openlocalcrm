package storage_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/storage"
)

func TestLocalStorageOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "crm-storage-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := storage.NewLocalStorage(tempDir)
	ctx := context.Background()

	content := []byte("Proposal PDF Attachment Data Content 12345")
	filename := "vertrag_muster.pdf"

	// 1. Save
	relPath, err := store.Save(ctx, filename, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("expected successful save, got: %v", err)
	}
	if relPath == "" {
		t.Fatalf("expected non-empty relative path")
	}

	// 2. Stat
	size, err := store.Stat(ctx, relPath)
	if err != nil {
		t.Fatalf("expected stat to succeed, got: %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), size)
	}

	// 3. Open & Read
	reader, err := store.Open(ctx, relPath)
	if err != nil {
		t.Fatalf("expected open to succeed, got: %v", err)
	}
	defer reader.Close()

	readData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed reading saved file: %v", err)
	}
	if string(readData) != string(content) {
		t.Fatalf("content mismatch, expected '%s', got '%s'", string(content), string(readData))
	}

	// 4. Path Traversal Protection
	if _, err := store.Open(ctx, "../../../etc/passwd"); err == nil {
		t.Fatalf("expected path traversal to fail with error")
	}

	// 5. Delete
	if err := store.Delete(ctx, relPath); err != nil {
		t.Fatalf("expected delete to succeed, got: %v", err)
	}

	// 6. Verify Deleted
	if _, err := store.Open(ctx, relPath); err != storage.ErrFileNotFound {
		t.Fatalf("expected ErrFileNotFound after deletion, got: %v", err)
	}
}
