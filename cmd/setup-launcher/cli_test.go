package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCLI_Help(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--help"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected help output to mention OpenLocalCRM, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "status") || !strings.Contains(buf.String(), "install") {
		t.Errorf("expected help to list subcommands, got: %s", buf.String())
	}
}

func TestRunCLI_Version(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--version"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected version output, got: %s", buf.String())
	}
}

func TestRunCLI_UnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"foobar"}, t.TempDir(), &buf)
	if !handled || code != 2 {
		t.Fatalf("expected handled=true code=2 for unknown command, got %v, %d", handled, code)
	}
}

func TestRunCLI_GuiHandled(t *testing.T) {
	var buf bytes.Buffer
	handled, _ := ExecuteCommand([]string{"gui"}, t.TempDir(), &buf)
	if handled {
		t.Errorf("expected gui command to return handled=false to launch server, got true")
	}
}

func TestRunCLI_Versions(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"versions"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	output := buf.String()
	if !strings.Contains(output, "Verfügbare Versionen") || !strings.Contains(output, "v0.9") {
		t.Errorf("expected versions output containing v0.9, got:\n%s", output)
	}
}

func TestRunCLI_InstallHelpHasVersion(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--help"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	output := buf.String()
	if !strings.Contains(output, "--version, --tag TAG") {
		t.Errorf("expected help output to mention --version / --tag flag, got:\n%s", output)
	}
}

