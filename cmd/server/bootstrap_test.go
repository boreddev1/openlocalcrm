package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBootstrapPasswordIsNotLoggedAndLandsInAFile: the secret must not reach the
// process log, and the file that holds it must be 0600.
func TestBootstrapPasswordIsNotLoggedAndLandsInAFile(t *testing.T) {
	dir := t.TempDir()
	var logs bytes.Buffer
	log.SetOutput(&logs)

	secret := "0123456789abcdef0123456789abcdef"
	require.NoError(t, writeBootstrapSecret(dir, "admin@example.de", secret))

	require.NotContains(t, logs.String(), secret, "the bootstrap password must never be logged")

	path := filepath.Join(dir, "bootstrap-admin.txt")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), secret)
}
