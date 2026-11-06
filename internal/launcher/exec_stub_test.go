package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// Environment markers understood by TestLauncherExecHelperProcess.
const (
	execHelperEnv       = "OPENLOCALCRM_LAUNCHER_EXEC_HELPER"
	execHelperStdoutEnv = "OPENLOCALCRM_LAUNCHER_EXEC_HELPER_STDOUT"
)

// TestLauncherExecHelperProcess is not a real test: it is the entry point for
// the stand-in processes created by newExecStub. The stub re-executes this test
// binary with only this test selected and the helper environment markers set;
// the child then prints the canned stdout the stub configured and exits 0, so
// Run/Output/CombinedOutput/Start+Wait keep working like a successful
// production command without ever invoking docker, wsl.exe or powershell.exe.
// In a normal `go test` run the environment guard is off and this function
// returns immediately, which is why it shows up as a trivial pass.
func TestLauncherExecHelperProcess(t *testing.T) {
	if os.Getenv(execHelperEnv) != "1" {
		return
	}
	if out := os.Getenv(execHelperStdoutEnv); out != "" {
		fmt.Print(out)
	}
	os.Exit(0)
}

// recordedExec is one command an engine created through the seam. The
// *exec.Cmd pointer is kept so tests can inspect fields the caller set after
// construction (Dir, Stdin, ...) while the recorded argv proves which command
// was supposed to run.
type recordedExec struct {
	argv []string
	cmd  *exec.Cmd
}

func (r recordedExec) joined() string {
	return strings.Join(r.argv, " ")
}

// execRecorder replaces Engine.execFn in tests. stdoutByArgv maps a
// space-joined argv (e.g. `docker ps -a --format {{.Names}}`) to the text the
// stand-in process should print, which lets tests fake daemon inventory for
// the dynamic cleanup sweeps in ResetAll or script successful steps of a
// restore. Only configured commands are actually spawned; every other command
// still gets recorded, but Run/Output/CombinedOutput fail immediately without
// starting any process — the same failure mode the engine must tolerate when
// docker is not installed at all, and why ResetAll swallows rm errors.
type execRecorder struct {
	mu           sync.Mutex
	stdoutByArgv map[string]string
	commands     []recordedExec
}

// errExecStubNotRun marks a recorded-but-not-spawned command.
var errExecStubNotRun = errors.New("launcher exec stub: command recorded, no process started")

func newExecStub(t *testing.T, stdoutByArgv map[string]string) (*execRecorder, func(ctx context.Context, name string, arg ...string) *exec.Cmd) {
	t.Helper()
	rec := &execRecorder{stdoutByArgv: stdoutByArgv}
	stub := func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		argv := append([]string{name}, arg...)

		var cmd *exec.Cmd
		if out, scripted := rec.stdoutByArgv[strings.Join(argv, " ")]; scripted {
			//nolint:gosec // G204: test-only stub; the executed "command" is this test binary itself
			cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLauncherExecHelperProcess$")
			cmd.Env = append(os.Environ(),
				execHelperEnv+"=1",
				execHelperStdoutEnv+"="+out,
			)
		} else {
			// Recorded for assertions, but running it fails instantly without
			// ever starting a process — nothing can touch the host daemon.
			cmd = &exec.Cmd{Args: argv}
			cmd.Err = errExecStubNotRun
		}

		rec.mu.Lock()
		rec.commands = append(rec.commands, recordedExec{argv: argv, cmd: cmd})
		rec.mu.Unlock()
		return cmd
	}
	return rec, stub
}

// recorded returns the argv of every issued command, in call order, joined by
// single spaces.
func (r *execRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c.joined())
	}
	return out
}

// mustFind fails the test unless a command with exactly this argv was issued
// and returns its record (including the *exec.Cmd for Dir/Stdin checks).
func (r *execRecorder) mustFind(t *testing.T, argv ...string) recordedExec {
	t.Helper()
	want := strings.Join(argv, " ")
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if c.joined() == want {
			return c
		}
	}
	t.Fatalf("expected command %q was never issued.\nrecorded commands:\n  %s", want, strings.Join(r.recordedLocked(), "\n  "))
	return recordedExec{}
}

// mustNotRecord fails the test if the exact argv was issued.
func (r *execRecorder) mustNotRecord(t *testing.T, argv ...string) {
	t.Helper()
	want := strings.Join(argv, " ")
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if c.joined() == want {
			t.Errorf("command %q must not be issued (outside the name-scoped sweep)", want)
			return
		}
	}
}

func (r *execRecorder) recordedLocked() []string {
	out := make([]string, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c.joined())
	}
	return out
}
