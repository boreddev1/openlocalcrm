package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/launcher"
)

func findAvailablePort(startPort int) int {
	for port := startPort; port < startPort+50; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return port
		}
	}
	return startPort
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, bsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func ResolveProjectBaseDir() string {
	// 1. If docker-compose.yml exists in current working dir, use it
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "."
	}
	// 2. Check executable directory and parent directory (e.g. when run from bin/)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if _, err := os.Stat(filepath.Join(exeDir, "docker-compose.yml")); err == nil {
			return exeDir
		}
		parentDir := filepath.Dir(exeDir)
		if _, err := os.Stat(filepath.Join(parentDir, "docker-compose.yml")); err == nil {
			return parentDir
		}

		// 3. Check known standard installation directories on Windows
		candidates := []string{
			"C:\\openlocalcrm",
			"C:\\mavalio",
			filepath.Join(os.Getenv("LOCALAPPDATA"), "openlocalcrm"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "mavalio"),
			filepath.Join(os.Getenv("ProgramFiles"), "OpenLocalCRM"),
			filepath.Join(os.Getenv("ProgramFiles"), "mavalio CRM"),
		}
		for _, c := range candidates {
			if c != "" {
				if _, err := os.Stat(filepath.Join(c, "docker-compose.yml")); err == nil {
					return c
				}
				if _, err := os.Stat(filepath.Join(c, ".env")); err == nil {
					return c
				}
			}
		}

		return exeDir
	}
	return "."
}

func main() {
	// Determine base directory (checks ., bin/.., and exeDir)
	baseDir := ResolveProjectBaseDir()
	if err := launcher.EnsureComposeAndCaddyFiles(baseDir); err != nil {
		log.Printf("[WARNING] EnsureComposeAndCaddyFiles: %v", err)
	}

	// Automatic file logging to openlocalcrm-setup.log in base directory
	logFilePath := filepath.Join(baseDir, "openlocalcrm-setup.log")
	if logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}

	log.Println("==================================================")
	log.Printf("  OpenLocalCRM — Windows Setup & Control Launcher")
	log.Printf("  Started at: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("  Base Dir:   %s", baseDir)
	log.Printf("  Log File:   %s", logFilePath)
	log.Println("==================================================")

	port := findAvailablePort(9099)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	serverURL := fmt.Sprintf("http://%s", addr)

	engine := launcher.NewEngine(baseDir)
	handler := launcher.NewServer(baseDir, engine)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Allow SSE / long requests
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Printf("[Server] Setup & Control Server listening on %s", serverURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server error: %v", err)
		}
	}()

	// Open browser automatically after brief delay
	go func() {
		time.Sleep(300 * time.Millisecond)
		log.Printf("[Browser] Opening %s in default browser...", serverURL)
		_ = openBrowser(serverURL)
	}()

	// Run Tray loop (Win32 tray on Windows, signal wait on other platforms)
	runTray(ctx, serverURL)

	log.Println("[Shutdown] Stopping launcher server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("[Shutdown] Done.")
}
