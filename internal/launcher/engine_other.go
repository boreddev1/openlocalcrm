//go:build !windows

package launcher

import (
	"context"
	"os/exec"
)

// execCommand is the production default for every command the launcher runs.
// Engine routes through its execFn seam (engine.go), which NewEngine wires to
// this function; tests replace only the per-engine seam so destructive flows
// never reach the host Docker daemon. Package-level helpers such as the
// preflight checks keep using this function directly.
func execCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...) //nolint:gosec // G702: command and args are chosen by the launcher itself, never taken from request input
}
