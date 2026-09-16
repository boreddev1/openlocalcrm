package launcher

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type SystemStatus struct {
	WSLInstalled    bool   `json:"wsl_installed"`
	WSLStatus       string `json:"wsl_status"`
	DockerInstalled bool   `json:"docker_installed"`
	DockerRunning   bool   `json:"docker_running"`
	DockerVersion   string `json:"docker_version"`
	Port80Free      bool   `json:"port_80_free"`
	Port8080Free    bool   `json:"port_8080_free"`
	SuggestedPort   int    `json:"suggested_port"`
}

func CheckPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func parseWSLOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "not installed") || strings.Contains(lower, "nicht installiert") || strings.Contains(lower, "keine installierte") {
		return false
	}
	if strings.Contains(lower, "default version") || strings.Contains(lower, "standardversion") || strings.Contains(lower, "running") || strings.Contains(lower, "kernel") || strings.Contains(lower, "wsl 2") || strings.Contains(lower, "wsl2") {
		return true
	}
	return false
}

func CheckWSLStatus(ctx context.Context) (bool, string) {
	if runtime.GOOS != "windows" {
		return true, "WSL not required on non-Windows host"
	}

	if _, err := exec.LookPath("wsl.exe"); err != nil {
		return false, "wsl.exe not found in PATH"
	}

	cmd := execCommand(ctx, "wsl.exe", "--status")
	out, err := cmd.CombinedOutput()
	outputStr := string(out)
	if err != nil {
		cmdFallback := execCommand(ctx, "wsl.exe", "-l", "-v")
		outFallback, errFallback := cmdFallback.CombinedOutput()
		if errFallback == nil && len(outFallback) > 0 {
			return true, strings.TrimSpace(string(outFallback))
		}
		return false, fmt.Sprintf("WSL status check failed: %v (%s)", err, strings.TrimSpace(outputStr))
	}

	installed := parseWSLOutput(outputStr)
	return installed, strings.TrimSpace(outputStr)
}

func TriggerWSLInstall(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WSL installation is only supported on Windows")
	}
	cmd := execCommand(ctx, "powershell.exe", "-NoProfile", "-Command", "Start-Process wsl -ArgumentList '--install --no-distribution' -Verb RunAs")
	return cmd.Run()
}

func TriggerDockerInstall(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Docker Desktop installation via winget is only supported on Windows")
	}
	cmd := execCommand(ctx, "powershell.exe", "-NoProfile", "-Command", "Start-Process winget -ArgumentList 'install -e --id Docker.DockerDesktop --accept-package-agreements --accept-source-agreements' -Verb RunAs")
	return cmd.Run()
}

func TriggerDockerStart(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Docker Desktop auto-start is only supported on Windows")
	}
	standardPath := filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "Docker Desktop.exe")
	cmd := execCommand(ctx, "powershell.exe", "-NoProfile", "-Command", fmt.Sprintf("Start-Process '%s'", standardPath))
	return cmd.Run()
}

func RunPreflightCheck(ctx context.Context) SystemStatus {
	status := SystemStatus{
		Port80Free:   CheckPortAvailable(80),
		Port8080Free: CheckPortAvailable(8080),
	}

	if status.Port80Free {
		status.SuggestedPort = 80
	} else if status.Port8080Free {
		status.SuggestedPort = 8080
	} else {
		status.SuggestedPort = 8000
	}

	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	wslInstalled, wslMsg := CheckWSLStatus(checkCtx)
	status.WSLInstalled = wslInstalled
	status.WSLStatus = wslMsg

	cmd := execCommand(checkCtx, "docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err == nil {
		status.DockerInstalled = true
		status.DockerRunning = true
		status.DockerVersion = strings.TrimSpace(string(out))
	} else {
		if _, lookErr := exec.LookPath("docker"); lookErr == nil {
			status.DockerInstalled = true
			status.DockerRunning = false
		} else {
			status.DockerInstalled = false
			status.DockerRunning = false
		}
	}

	return status
}
