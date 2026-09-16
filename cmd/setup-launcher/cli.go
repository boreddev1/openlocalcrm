package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/launcher"
)

func generateRandomPassword(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "AdminPass" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	return hex.EncodeToString(bytes)
}

func extractDirFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--dir=") {
			return strings.TrimPrefix(args[i], "--dir=")
		}
	}
	return ""
}

// RunCLI executes command with standard output
func RunCLI(args []string, baseDir string) (bool, int) {
	return ExecuteCommand(args, baseDir, os.Stdout)
}

// ExecuteCommand routes and executes a CLI subcommand
func ExecuteCommand(args []string, baseDir string, out io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}

	cmd := args[0]
	switch cmd {
	case "help", "--help", "-h":
		printHelp(out)
		return true, 0
	case "version", "--version", "-v":
		printVersion(out)
		return true, 0
	case "gui":
		return false, 0
	case "status":
		return true, handleStatus(args[1:], baseDir, out)
	case "install":
		return true, handleInstall(args[1:], baseDir, out)
	case "start":
		return true, handleStart(baseDir, out)
	case "stop":
		return true, handleStop(baseDir, out)
	case "restart":
		return true, handleRestart(baseDir, out)
	case "down":
		return true, handleDown(baseDir, out)
	case "update":
		return true, handleUpdate(baseDir, out)
	case "versions":
		return true, handleVersionsList(baseDir, out)
	case "backup":
		return true, handleBackup(args[1:], baseDir, out)
	case "reset":
		return true, handleReset(args[1:], baseDir, out)
	case "admin":
		return true, handleAdmin(args[1:], baseDir, out)
	default:
		fmt.Fprintf(out, "Unbekannter Befehl: %s\nNutzen Sie 'openlocalcrm --help' für verfügbare Befehle.\n", cmd)
		return true, 2
	}
}

func printVersion(out io.Writer) {
	commit := launcher.BuildCommit
	if commit == "" {
		commit = "dev"
	}
	fmt.Fprintf(out, "OpenLocalCRM Setup & Control Center v3.0 (Commit: %s)\n", commit)
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `==============================================================================
  OpenLocalCRM — Setup & Control Center
==============================================================================

NUTZUNG:
  openlocalcrm [BEFEHL] [OPTIONEN]
  openlocalcrm gui                  Web-Steuerungszentrale im Browser öffnen

BEFEHLE:
  status [--json]                   Status der Docker-Container und CRM-Dienste prüfen
  install [OPTIONEN]                OpenLocalCRM installieren und initialisieren
  start                             Alle CRM-Container starten
  stop                              Laufende CRM-Container anhalten
  restart                           CRM-Container neu starten
  down                              Container stoppen und Netzwerk freigeben
  update                            CRM-Dateien & Container auf neueste Version aktualisieren
  versions                          Verfügbare Release-Tags und Versionen anzeigen
  backup [create|list|restore FILE] Datenbank-Sicherungen erstellen, anzeigen oder einspielen
  reset [-f, --force]               Vollständiger Factory Reset (Datenbank, Volumes, .env löschen)
  admin password EMAIL NEUES_PW     Administrator-Passwort in PostgreSQL zurücksetzen
  --help, -h                        Diese Hilfe anzeigen
  --version, -v                     Version anzeigen

INSTALLATIONS-OPTIONEN:
  --version, --tag TAG              Zu installierende Version / Release-Tag (Standard: v0.9)
  --list-versions                   Alle verfügbaren Release-Tags von GitHub anzeigen
  --port PORT                       Web-Port festlegen (Standard: 80, Ausweich: 8080)
  --admin-email EMAIL               E-Mail-Adresse für das Admin-Konto (Standard: admin@openlocalcrm.local)
  --admin-password PASSWORT         Initiales Passwort (wird automatisch generiert falls weggelassen)
  --ai-provider ANBIETER            KI-Anbindung (ollama, openai, none) (Standard: ollama)
  --ai-url URL                      URL für Ollama oder OpenAI kompatible API
  --ai-model MODELL                 KI-Modellname (z. B. gemma2:9b)
  --ai-key SCHLÜSSEL                API-Schlüssel für OpenAI
  --demo                            Demo-Modus mit synthetischen Daten aktivieren
  -y, --yes, --non-interactive      Bestätigungsabfragen überspringen
  --dir PFAD                        Zielverzeichnis für CRM-Dateien festlegen`)
}

func handleStatus(args []string, baseDir string, out io.Writer) int {
	jsonOutput := false
	for _, a := range args {
		if a == "--json" {
			jsonOutput = true
		}
	}

	engine := launcher.NewEngine(baseDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := engine.GetContainers(ctx)
	cfg, hasCfg := launcher.ReadExistingConfig(baseDir)
	port := 80
	if hasCfg && cfg.Port > 0 {
		port = cfg.Port
	}

	appURL := fmt.Sprintf("http://localhost:%d", port)
	guiURL := "http://localhost:9099"

	if jsonOutput {
		payload := map[string]any{
			"base_dir":       baseDir,
			"has_config":     hasCfg,
			"port":           port,
			"app_url":        appURL,
			"launcher_url":   guiURL,
			"containers":     containers,
			"docker_running": err == nil,
		}
		if err != nil {
			payload["error"] = err.Error()
		}
		_ = json.NewEncoder(out).Encode(payload)
		if err != nil {
			return 1
		}
		return 0
	}

	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintln(out, "  OpenLocalCRM Systemstatus")
	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintf(out, "Projekt-Verzeichnis: %s\n", baseDir)
	fmt.Fprintf(out, "CRM Web-Adresse:     %s\n", appURL)
	fmt.Fprintf(out, "Launcher Web-GUI:    %s\n\n", guiURL)

	if err != nil {
		fmt.Fprintf(out, "⚠️  Docker-Status: Fehler beim Abfragen der Container (%v)\n", err)
		return 1
	}

	if len(containers) == 0 {
		fmt.Fprintln(out, "ℹ️  Keine laufenden OpenLocalCRM-Container gefunden.")
		fmt.Fprintln(out, "   Nutzen Sie 'openlocalcrm start' oder 'openlocalcrm install'.")
		return 0
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DIENST\tCONTAINER\tSTATUS\tHEALTH")
	fmt.Fprintln(w, "------\t---------\t------\t------")
	for _, c := range containers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Service, c.Name, c.State, c.Health)
	}
	_ = w.Flush()
	fmt.Fprintln(out)
	return 0
}

func handleInstall(args []string, baseDir string, out io.Writer) int {
	cfg := launcher.SetupConfig{
		AdminEmail: "admin@openlocalcrm.local",
		Port:       80,
		AIProvider: "ollama",
		AIBaseURL:  "http://localhost:11434",
		AIModel:    "gemma2:9b",
	}

	nonInteractive := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-y" || arg == "--yes" || arg == "--non-interactive":
			nonInteractive = true
		case arg == "--demo":
			cfg.IsDemoMode = true
		case arg == "--port" && i+1 < len(args):
			i++
			if p, err := strconv.Atoi(args[i]); err == nil && p > 0 {
				cfg.Port = p
			}
		case strings.HasPrefix(arg, "--port="):
			if p, err := strconv.Atoi(strings.TrimPrefix(arg, "--port=")); err == nil && p > 0 {
				cfg.Port = p
			}
		case arg == "--admin-email" && i+1 < len(args):
			i++
			cfg.AdminEmail = args[i]
		case strings.HasPrefix(arg, "--admin-email="):
			cfg.AdminEmail = strings.TrimPrefix(arg, "--admin-email=")
		case arg == "--admin-password" && i+1 < len(args):
			i++
			cfg.AdminPassword = args[i]
		case strings.HasPrefix(arg, "--admin-password="):
			cfg.AdminPassword = strings.TrimPrefix(arg, "--admin-password=")
		case arg == "--ai-provider" && i+1 < len(args):
			i++
			cfg.AIProvider = args[i]
		case strings.HasPrefix(arg, "--ai-provider="):
			cfg.AIProvider = strings.TrimPrefix(arg, "--ai-provider=")
		case arg == "--ai-url" && i+1 < len(args):
			i++
			cfg.AIBaseURL = args[i]
		case strings.HasPrefix(arg, "--ai-url="):
			cfg.AIBaseURL = strings.TrimPrefix(arg, "--ai-url=")
		case arg == "--ai-model" && i+1 < len(args):
			i++
			cfg.AIModel = args[i]
		case strings.HasPrefix(arg, "--ai-model="):
			cfg.AIModel = strings.TrimPrefix(arg, "--ai-model=")
		case arg == "--ai-key" && i+1 < len(args):
			i++
			cfg.AIAPIKey = args[i]
		case strings.HasPrefix(arg, "--ai-key="):
			cfg.AIAPIKey = strings.TrimPrefix(arg, "--ai-key=")
		case arg == "--list-versions" || arg == "--versions":
			return handleVersionsList(baseDir, out)
		case (arg == "--version" || arg == "--tag" || arg == "--release") && i+1 < len(args):
			i++
			cfg.Version = args[i]
		case strings.HasPrefix(arg, "--version="):
			cfg.Version = strings.TrimPrefix(arg, "--version=")
		case strings.HasPrefix(arg, "--tag="):
			cfg.Version = strings.TrimPrefix(arg, "--tag=")
		case strings.HasPrefix(arg, "--release="):
			cfg.Version = strings.TrimPrefix(arg, "--release=")
		}
	}

	if cfg.Version == "" {
		cfg.Version = "v0.9"
	}

	if cfg.AdminPassword == "" {
		cfg.AdminPassword = generateRandomPassword(16)
	}

	if !launcher.CheckPortAvailable(cfg.Port) {
		if cfg.Port == 80 && launcher.CheckPortAvailable(8080) {
			fmt.Fprintln(out, "⚠️  Port 80 belegt. Weiche automatisch auf Port 8080 aus.")
			cfg.Port = 8080
		}
	}

	if launcher.IsAlreadyInstalled(baseDir) && !nonInteractive {
		fmt.Fprintln(out, "📦 In diesem Verzeichnis existiert bereits eine .env-Konfiguration.")
		fmt.Fprintln(out, "   Die Konfiguration wird aktualisiert und Container neu gestartet.")
	}

	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintln(out, "  OpenLocalCRM — Installation & Bereitstellung")
	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintf(out, "Zielverzeichnis: %s\n", baseDir)
	fmt.Fprintf(out, "Version:         %s\n", cfg.Version)
	fmt.Fprintf(out, "Web-Port:        %d\n", cfg.Port)
	fmt.Fprintf(out, "Admin-Konto:     %s\n", cfg.AdminEmail)
	fmt.Fprintf(out, "KI-Anbindung:    %s\n", cfg.AIProvider)
	fmt.Fprintln(out, "------------------------------------------------------------------------------")

	// Ensure project files exist for selected version if missing
	updater := launcher.NewUpdater(baseDir)
	if err := updater.EnsureProjectFilesForVersion(context.Background(), cfg.Version, false, nil); err != nil {
		fmt.Fprintf(out, "⚠️  Hinweis zur Projektdateien-Bereitstellung: %v\n", err)
	}

	// Write docker-compose, caddyfile, .env
	if err := launcher.EnsureComposeAndCaddyFiles(baseDir); err != nil {
		fmt.Fprintf(out, "⚠️  Warnung beim Erstellen der Basisdateien: %v\n", err)
	}
	if err := launcher.WriteConfigAndDirectories(baseDir, cfg); err != nil {
		fmt.Fprintf(out, "❌ Fehler beim Schreiben der Konfiguration: %v\n", err)
		return 1
	}

	engine := launcher.NewEngine(baseDir)
	logChan := make(chan string, 100)
	done := make(chan error, 1)

	go func() {
		for line := range logChan {
			fmt.Fprintln(out, line)
		}
	}()

	go func() {
		done <- engine.Up(context.Background(), logChan)
		close(logChan)
	}()

	if err := <-done; err != nil {
		fmt.Fprintf(out, "\n❌ Fehler beim Starten von Docker Compose: %v\n", err)
		return 1
	}

	appURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	fmt.Fprintln(out, "\n==============================================================================")
	fmt.Fprintln(out, "  ✅ OpenLocalCRM wurde erfolgreich installiert und gestartet!")
	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintf(out, "  Web-Adresse:     %s\n", appURL)
	fmt.Fprintf(out, "  Administrator:   %s\n", cfg.AdminEmail)
	fmt.Fprintf(out, "  Passwort:        %s\n", cfg.AdminPassword)
	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintln(out, "  Tipp: Speichern Sie diese Zugangsdaten an einem sicheren Ort.")
	fmt.Fprintln(out)
	return 0
}

func handleStart(baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	fmt.Fprintln(out, "[Start] Fahre OpenLocalCRM-Container hoch...")
	logChan := make(chan string, 50)
	done := make(chan error, 1)
	go func() {
		for line := range logChan {
			fmt.Fprintln(out, line)
		}
	}()
	go func() {
		done <- engine.Up(context.Background(), logChan)
		close(logChan)
	}()

	if err := <-done; err != nil {
		fmt.Fprintf(out, "❌ Fehler beim Starten: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Container erfolgreich gestartet.")
	return 0
}

func handleStop(baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	fmt.Fprintln(out, "[Stop] Halte OpenLocalCRM-Container an...")
	if err := engine.Stop(context.Background()); err != nil {
		fmt.Fprintf(out, "❌ Fehler beim Anhalten: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Container angehalten.")
	return 0
}

func handleRestart(baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	fmt.Fprintln(out, "[Restart] Starte OpenLocalCRM-Container neu...")
	if err := engine.Restart(context.Background()); err != nil {
		fmt.Fprintf(out, "❌ Fehler beim Neustart: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Container erfolgreich neu gestartet.")
	return 0
}

func handleDown(baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	fmt.Fprintln(out, "[Down] Stoppe Container und gebe Netzwerk frei...")
	if err := engine.Down(context.Background()); err != nil {
		fmt.Fprintf(out, "❌ Fehler: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Container gestoppt.")
	return 0
}

func handleUpdate(baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	updater := launcher.NewUpdater(baseDir)

	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintln(out, "  OpenLocalCRM — System-Aktualisierung")
	fmt.Fprintln(out, "==============================================================================")

	check, err := updater.CheckForUpdate(launcher.BuildCommit)
	if err != nil {
		fmt.Fprintf(out, "⚠️  Update-Prüfung fehlgeschlagen: %v. Versuche Aktualisierung dennoch...\n", err)
	} else if !check.HasUpdate && !check.RateLimited {
		fmt.Fprintf(out, "ℹ️  System ist bereits auf dem neuesten Stand (%s).\n", check.CurrentCommit)
	}

	logChan := make(chan string, 100)
	done := make(chan error, 1)

	go func() {
		for line := range logChan {
			fmt.Fprintln(out, line)
		}
	}()

	go func() {
		done <- updater.ExecuteUpdate(context.Background(), engine, logChan)
		close(logChan)
	}()

	if err := <-done; err != nil {
		fmt.Fprintf(out, "\n❌ Aktualisierung fehlgeschlagen: %v\n", err)
		return 1
	}

	fmt.Fprintln(out, "\n✅ OpenLocalCRM wurde erfolgreich aktualisiert!")
	return 0
}

func handleBackup(args []string, baseDir string, out io.Writer) int {
	engine := launcher.NewEngine(baseDir)
	subAction := "create"
	if len(args) > 0 {
		subAction = args[0]
	}

	switch subAction {
	case "create":
		fmt.Fprintln(out, "[Backup] Erstelle Datenbanksicherung...")
		filename, err := engine.CreateBackup(context.Background())
		if err != nil {
			fmt.Fprintf(out, "❌ Sicherung fehlgeschlagen: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "✅ Sicherung erfolgreich erstellt: %s\n", filename)
		return 0

	case "list":
		backups, err := engine.ListBackups()
		if err != nil {
			fmt.Fprintf(out, "❌ Fehler beim Abfragen der Sicherungen: %v\n", err)
			return 1
		}
		if len(backups) == 0 {
			fmt.Fprintln(out, "ℹ️  Keine Datenbanksicherungen in backups/ vorhanden.")
			return 0
		}
		w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "DATEI\tGRÖSSE\tERSTELLT AM\tSCHEMA")
		fmt.Fprintln(w, "-----\t------\t-----------\t------")
		for _, b := range backups {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Filename, b.SizeFormatted, b.CreatedAt.Format("2006-01-02 15:04:05"), b.SchemaVersion)
		}
		_ = w.Flush()
		return 0

	case "restore":
		if len(args) < 2 {
			fmt.Fprintln(out, "Fehler: Dateiname der Sicherung erforderlich (z.B. openlocalcrm backup restore DATEI.sql)")
			return 2
		}
		backupFile := args[1]
		fmt.Fprintf(out, "[Restore] Stelle Datenbank aus Sicherung '%s' wieder her...\n", backupFile)
		logChan := make(chan string, 100)
		done := make(chan error, 1)
		go func() {
			for line := range logChan {
				fmt.Fprintln(out, line)
			}
		}()
		go func() {
			done <- engine.RestoreBackup(context.Background(), backupFile, logChan)
			close(logChan)
		}()

		if err := <-done; err != nil {
			fmt.Fprintf(out, "❌ Wiederherstellung fehlgeschlagen: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, "✅ Wiederherstellung erfolgreich abgeschlossen.")
		return 0

	default:
		fmt.Fprintf(out, "Unbekannte Backup-Aktion: %s (erlaubt: create, list, restore)\n", subAction)
		return 2
	}
}

func handleReset(args []string, baseDir string, out io.Writer) int {
	force := false
	for _, a := range args {
		if a == "-f" || a == "--force" {
			force = true
		}
	}

	if !force {
		fmt.Fprintln(out, "⚠️  WARNUNG: Ein Factory Reset löscht alle CRM-Datenbanken, Docker-Volumes und lokalen Konfigurationen!")
		fmt.Fprint(out, "Möchten Sie wirklich fortfahren? [j/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(out, "\nAbgebrochen (keine Bestätigung erhalten).")
			return 1
		}
		trimmed := strings.ToLower(strings.TrimSpace(input))
		if trimmed != "j" && trimmed != "ja" && trimmed != "y" && trimmed != "yes" {
			fmt.Fprintln(out, "Abgebrochen.")
			return 1
		}
	}

	engine := launcher.NewEngine(baseDir)
	logChan := make(chan string, 100)
	done := make(chan error, 1)

	go func() {
		for line := range logChan {
			fmt.Fprintln(out, line)
		}
	}()

	go func() {
		done <- engine.ResetAll(context.Background(), logChan)
		close(logChan)
	}()

	if err := <-done; err != nil {
		fmt.Fprintf(out, "❌ Reset fehlgeschlagen: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Factory Reset abgeschlossen. Das System kann sauber neu installiert werden.")
	return 0
}

func handleAdmin(args []string, baseDir string, out io.Writer) int {
	if len(args) < 3 || args[0] != "password" {
		fmt.Fprintln(out, "Nutzung: openlocalcrm admin password <EMAIL> <NEUES_PASSWORT>")
		return 2
	}

	email := args[1]
	password := args[2]

	engine := launcher.NewEngine(baseDir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(out, "[Admin] Setze Passwort für Administrator '%s' zurück...\n", email)
	if err := engine.ResetAdminPassword(ctx, email, password); err != nil {
		fmt.Fprintf(out, "❌ Fehler beim Zurücksetzen des Passworts: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "✅ Passwort für '%s' erfolgreich aktualisiert.\n", email)
	return 0
}

func handleVersionsList(baseDir string, out io.Writer) int {
	updater := launcher.NewUpdater(baseDir)
	resp := updater.GetAvailableVersions(context.Background())

	fmt.Fprintln(out, "==============================================================================")
	fmt.Fprintln(out, "  OpenLocalCRM — Verfügbare Versionen & Release-Tags")
	fmt.Fprintln(out, "==============================================================================")
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TAG / ZWEIG\tSTATUS\tCOMMIT")
	fmt.Fprintln(w, "-----------\t------\t------")
	for _, v := range resp.Versions {
		status := ""
		if v.IsLatest {
			status = "Neueste / Empfohlen"
		} else if v.Tag == "main" {
			status = "Edge / Entwicklungszweig"
		} else {
			status = "Release"
		}
		if v.Tag == resp.CurrentVersion {
			status += " (installiert)"
		}
		commit := v.Commit
		if commit == "" {
			commit = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", v.Tag, status, commit)
	}
	_ = w.Flush()
	fmt.Fprintln(out)
	return 0
}

