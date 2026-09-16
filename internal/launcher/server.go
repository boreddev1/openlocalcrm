package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openlocalcrm/openlocalcrm/internal/launcher/ui"
)

type Server struct {
	baseDir string
	engine  *Engine

	mu   sync.RWMutex
	logs []string
}

func NewServer(baseDir string, engine *Engine) http.Handler {
	s := &Server{
		baseDir: baseDir,
		engine:  engine,
		logs:    make([]string, 0),
	}

	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/preflight", s.handlePreflight)
	mux.HandleFunc("/api/ai/test", s.handleAITest)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/control/", s.handleControl)
	mux.HandleFunc("/api/backup", s.handleBackup)
	mux.HandleFunc("/api/backups", s.handleBackupsList)
	mux.HandleFunc("/api/backup/upload", s.handleBackupUpload)
	mux.HandleFunc("/api/backup/download", s.handleBackupDownload)
	mux.HandleFunc("/api/backup/restore", s.handleBackupRestore)
	mux.HandleFunc("/api/admin/reset-password", s.handleResetAdminPassword)
	mux.HandleFunc("/api/install/", s.handleInstall)
	mux.HandleFunc("/api/logs", s.handleLogs)

	// Embedded Static UI
	mux.Handle("/", ui.Handler())

	return mux
}

func (s *Server) appendLog(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, line)
	if len(s.logs) > 1000 {
		s.logs = s.logs[len(s.logs)-1000:]
	}
}

func (s *Server) getLogs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]string, len(s.logs))
	copy(res, s.logs)
	return res
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"lines": s.getLogs(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	installed, reason := DetectExistingInstallation(r.Context(), s.baseDir, s.engine)
	existingCfg, hasConfig := ReadExistingConfig(s.baseDir)
	containers, _ := s.engine.GetContainers(r.Context())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"installed":       installed,
		"reason":          reason,
		"has_config":      hasConfig,
		"existing_config": existingCfg,
		"base_dir":        s.baseDir,
		"containers":      containers,
	})
}

func (s *Server) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := RunPreflightCheck(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleAITest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AIProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	resp := ProbeAIConnection(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg SetupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "Invalid setup configuration", http.StatusBadRequest)
		return
	}

	if err := WriteConfigAndDirectories(s.baseDir, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	logChan := make(chan string, 100)
	go func() {
		for line := range logChan {
			s.appendLog(line)
			log.Println("[Compose]", line)
		}
	}()

	go func() {
		defer close(logChan)
		s.appendLog("[Setup] Starte Docker Compose Stack...")
		log.Println("[Setup] Starting docker compose up -d...")

		err := s.engine.Up(context.Background(), logChan)
		if err != nil {
			errLine := fmt.Sprintf("[FEHLER] docker compose up fehlgeschlagen: %v", err)
			s.appendLog(errLine)
			log.Printf("[Setup Error] %s", errLine)
		} else {
			s.appendLog("[SUCCESS] Alle Container wurden erfolgreich gestartet!")
			log.Println("[Setup] All containers started successfully.")
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := strings.TrimPrefix(r.URL.Path, "/api/control/")
	var err error

	switch action {
	case "start":
		logChan := make(chan string, 50)
		go func() {
			for line := range logChan {
				s.appendLog(line)
				log.Println("[Compose]", line)
			}
		}()
		go func() {
			defer close(logChan)
			_ = s.engine.Up(context.Background(), logChan)
		}()
	case "stop":
		err = s.engine.Stop(context.Background())
	case "restart":
		err = s.engine.Restart(context.Background())
	case "down":
		err = s.engine.Down(context.Background())
	case "update":
		logChan := make(chan string, 100)
		go func() {
			for line := range logChan {
				s.appendLog(line)
				log.Println("[Update]", line)
			}
		}()
		go func() {
			defer close(logChan)
			if err := s.engine.Update(context.Background(), logChan); err != nil {
				s.appendLog(fmt.Sprintf("[FEHLER] Update fehlgeschlagen: %v", err))
			}
		}()
	case "reset":
		logChan := make(chan string, 100)
		done := make(chan struct{})
		go func() {
			for line := range logChan {
				s.appendLog(line)
				log.Println("[Reset]", line)
			}
			close(done)
		}()
		err = s.engine.ResetAll(context.Background(), logChan)
		close(logChan)
		<-done
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filename, err := s.engine.CreateBackup(r.Context())
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"filename": filename,
	})
}

func (s *Server) handleBackupsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	backups, err := s.engine.ListBackups()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"backups": backups,
	})
}

func (s *Server) handleBackupUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(500 << 20); err != nil {
		http.Error(w, fmt.Sprintf("failed parsing upload: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("backup_file")
	if err != nil {
		http.Error(w, "missing backup_file field in multipart form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	cleanName := filepath.Base(header.Filename)
	if !strings.HasSuffix(strings.ToLower(cleanName), ".sql") {
		http.Error(w, "only .sql backup files are allowed", http.StatusBadRequest)
		return
	}

	backupDir := filepath.Join(s.baseDir, "backups")
	_ = os.MkdirAll(backupDir, 0755)
	destPath := filepath.Join(backupDir, cleanName)

	destFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create destination file: %v", err), http.StatusInternalServerError)
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		http.Error(w, fmt.Sprintf("failed saving backup file: %v", err), http.StatusInternalServerError)
		return
	}

	s.appendLog(fmt.Sprintf("[Backup] Uploaded backup file %s", cleanName))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"filename": cleanName,
	})
}

func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filename := r.URL.Query().Get("file")
	cleanName := filepath.Base(filename)
	if cleanName != filename || cleanName == "." || cleanName == ".." || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "invalid filename: directory traversal detected", http.StatusBadRequest)
		return
	}

	backupPath := filepath.Join(s.baseDir, "backups", cleanName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		persistentPath := filepath.Join(GetPersistentBackupDir(), cleanName)
		if _, pErr := os.Stat(persistentPath); pErr == nil {
			backupPath = persistentPath
		} else {
			http.Error(w, "backup file not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, cleanName))
	w.Header().Set("Content-Type", "application/sql")
	http.ServeFile(w, r, backupPath)
}

type RestoreRequest struct {
	Filename string `json:"filename"`
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		http.Error(w, "invalid request: filename is required", http.StatusBadRequest)
		return
	}

	logChan := make(chan string, 100)
	go func() {
		for line := range logChan {
			s.appendLog(line)
			log.Println("[Restore]", line)
		}
	}()

	go func() {
		defer close(logChan)
		if err := s.engine.RestoreBackup(context.Background(), req.Filename, logChan); err != nil {
			s.appendLog(fmt.Sprintf("[FEHLER] Restore fehlgeschlagen: %v", err))
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := strings.TrimPrefix(r.URL.Path, "/api/install/")
	var err error

	switch target {
	case "wsl":
		s.appendLog("[Install] Triggering elevated WSL2 installation...")
		err = TriggerWSLInstall(r.Context())
	case "docker":
		s.appendLog("[Install] Triggering elevated Docker Desktop installation via winget...")
		err = TriggerDockerInstall(r.Context())
	case "start-docker":
		s.appendLog("[Install] Triggering Docker Desktop launch...")
		err = TriggerDockerStart(r.Context())
	default:
		http.Error(w, "Unknown install target", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

type ResetPasswordRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleResetAdminPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Ungültige Anfrage", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Email) == "" {
		envMap := ReadExistingEnvMap(s.baseDir)
		if existingEmail := envMap["INITIAL_ADMIN_EMAIL"]; existingEmail != "" {
			req.Email = existingEmail
		} else {
			req.Email = "admin@openlocalcrm.local"
		}
	}
	if strings.TrimSpace(req.Password) == "" {
		http.Error(w, "Passwort darf nicht leer sein", http.StatusBadRequest)
		return
	}

	s.appendLog(fmt.Sprintf("[Admin] Setze Administrator-Passwort für %s zurück...", req.Email))
	if err := s.engine.ResetAdminPassword(r.Context(), req.Email, req.Password); err != nil {
		s.appendLog(fmt.Sprintf("[FEHLER] Passwort-Reset fehlgeschlagen: %v", err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.appendLog(fmt.Sprintf("[SUCCESS] Admin-Passwort für %s erfolgreich aktualisiert!", req.Email))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": fmt.Sprintf("Passwort für %s erfolgreich aktualisiert!", req.Email),
	})
}
