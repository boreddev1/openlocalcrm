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

func main() {
	// If CLI arguments are provided, dispatch directly to CLI runner
	if len(os.Args) > 1 && os.Args[1] != "gui" {
		dirFlag := extractDirFlag(os.Args[1:])
		baseDir := ResolveProjectBaseDir(dirFlag)
		handled, exitCode := RunCLI(os.Args[1:], baseDir)
		if handled {
			os.Exit(exitCode)
		}
	}

	// Determine base directory (checks ., bin/.., and exeDir)
	baseDir := ResolveProjectBaseDir("")
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
	log.Printf("  OpenLocalCRM — Setup & Control Center")
	log.Printf("  Started at: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("  Base Dir:   %s", baseDir)
	log.Printf("  Log File:   %s", logFilePath)
	log.Println("==================================================")

	// Clean up any old executable backups from previous updates
	updater := launcher.NewUpdater(baseDir)
	updater.CleanupStaleOldExecutables()

	port := findAvailablePort(9099)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	serverURL := fmt.Sprintf("http://%s", addr)

	engine := launcher.NewEngine(baseDir)

	var srv *http.Server

	restartFunc := func(newExe string) {
		log.Println("[Restart] Fahre bestehenden Server geordnet herunter und gebe Port frei...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if srv != nil {
			_ = srv.Shutdown(shutdownCtx)
		}

		exeToRun := newExe
		if exeToRun == "" {
			if exe, err := os.Executable(); err == nil {
				exeToRun = exe
			}
		}

		if exeToRun != "" {
			log.Printf("[Restart] Starte neuen Launcher-Prozess: %s", exeToRun)
			cmd := exec.Command(exeToRun)
			cmd.Dir = baseDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if startErr := cmd.Start(); startErr != nil {
				log.Printf("[FEHLER] Konnte neuen Prozess nicht starten: %v", startErr)
			}
		}
		os.Exit(0)
	}

	handler := launcher.NewServerWithUpdater(baseDir, engine, updater, restartFunc)

	srv = &http.Server{
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

	isHeadlessLinux := runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""

	// Open browser automatically after brief delay (unless headless Linux)
	go func() {
		time.Sleep(300 * time.Millisecond)
		if isHeadlessLinux {
			log.Println("[Headless] Keine grafische Anzeige erkannt ($DISPLAY unset).")
			log.Printf("[Headless] Web-Oberfläche erreichbar unter: %s (z. B. via SSH-Tunnel).", serverURL)
			log.Println("[Headless] Nutzen Sie 'openlocalcrm --help' für die Steuerung im Terminal.")
			return
		}
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
