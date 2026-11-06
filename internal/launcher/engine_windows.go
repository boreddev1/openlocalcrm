//go:build windows

package launcher

import (
	"context"
	"os/exec"
	"syscall"
)

// execCommand is the production default for every command the launcher runs
// on Windows (hidden console window). Engine routes through its execFn seam
// (engine.go), which NewEngine wires to this function; tests replace only the
// per-engine seam so destructive flows never reach the host Docker daemon.
func execCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}
