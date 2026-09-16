//go:build !windows

package launcher

import (
	"context"
	"os/exec"
)

func execCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
