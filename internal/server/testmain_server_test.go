package server_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain pins the deployment configuration directory for this whole test
// binary to a temporary directory (issue #122).
//
// The router builds its settings handler without an explicit base directory, so
// the handler fell back to the process working directory — which for a Go test
// binary is the package directory. TestSettingsEndpoints POSTs
// /api/v1/settings/import, and that wrote a freshly generated
// "Auto-generated Production Configuration" .env (database password,
// DATABASE_URL with credentials, connector API token, admin password) directly
// into internal/server/. Only the `.env` filename rule in .gitignore kept it out
// of `git status`, so the next `git add -A` after a test run could have committed it.
//
// Unset $CONFIG_DIR keeps the historic working-directory behavior for real
// deployments; tests must never rely on it, which is why it is pinned here
// instead of per test — every route table in this package builds a router.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "openlocalcrm-server-testconfig-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "server: failed to create temp config directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("CONFIG_DIR", dir); err != nil {
		fmt.Fprintf(os.Stderr, "server: failed to set CONFIG_DIR: %v\n", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()

	// Guard the invariant itself: no .env may ever appear in the package tree.
	if _, statErr := os.Stat(".env"); statErr == nil {
		cwd, _ := os.Getwd()
		fmt.Fprintf(os.Stderr, "server: a test wrote .env into the package directory %s (issue #122)\n", filepath.Clean(cwd))
		code = 1
	}

	os.RemoveAll(dir)
	os.Exit(code)
}
