package launcher

import (
	"context"
	"net"
	"testing"
)

func TestCheckPortAvailable(t *testing.T) {
	// Bind to an arbitrary available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Port should NOT be available while listening
	if CheckPortAvailable(port) {
		t.Errorf("expected port %d to be unavailable while listener is active", port)
	}

	// Close listener, port should become available
	listener.Close()
	if !CheckPortAvailable(port) {
		t.Errorf("expected port %d to be available after listener closed", port)
	}
}

func TestRunPreflightCheck(t *testing.T) {
	status := RunPreflightCheck(context.Background())
	if status.SuggestedPort != 80 && status.SuggestedPort != 8080 && status.SuggestedPort != 8000 {
		t.Errorf("expected suggested port 80, 8080, or 8000, got %d", status.SuggestedPort)
	}
}

func TestParseWSLStatus(t *testing.T) {
	cases := []struct {
		output    string
		installed bool
	}{
		{"Default Version: 2\nWindows Subsystem for Linux is running.", true},
		{"WSL is not installed.", false},
		{"", false},
	}

	for _, c := range cases {
		installed := parseWSLOutput(c.output)
		if installed != c.installed {
			t.Errorf("parseWSLOutput(%q) = %v; want %v", c.output, installed, c.installed)
		}
	}
}

func TestCheckWSLStatus(t *testing.T) {
	installed, msg := CheckWSLStatus(context.Background())
	if !installed && msg == "" {
		t.Errorf("expected non-empty message when installed is false")
	}
}

func TestTriggerInstallFunctions_NonWindows(t *testing.T) {
	// If running on macOS or Linux, triggers should return descriptive error
	if err := TriggerWSLInstall(context.Background()); err == nil {
		t.Errorf("expected error running TriggerWSLInstall on non-windows platform")
	}
	if err := TriggerDockerInstall(context.Background()); err == nil {
		t.Errorf("expected error running TriggerDockerInstall on non-windows platform")
	}
	if err := TriggerDockerStart(context.Background()); err == nil {
		t.Errorf("expected error running TriggerDockerStart on non-windows platform")
	}
}

