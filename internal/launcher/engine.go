package launcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

type ContainerInfo struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Health  string `json:"health"`
}

type BackupInfo struct {
	Filename      string    `json:"filename"`
	SizeBytes     int64     `json:"size_bytes"`
	SizeFormatted string    `json:"size_formatted"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion string    `json:"schema_version,omitempty"`
}

type DockerInspectItem struct {
	Name  string `json:"Name"`
	State struct {
		Status string `json:"Status"`
		Health struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
		Env    []string          `json:"Env"`
	} `json:"Config"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func ParseDockerInspectJSON(data []byte) ([]DockerInspectItem, error) {
	var list []DockerInspectItem
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func ExtractEnvFromInspect(items []DockerInspectItem) map[string]string {
	env := make(map[string]string)
	for _, item := range items {
		cleanName := strings.TrimPrefix(item.Name, "/")
		if strings.Contains(cleanName, "crm-db") || strings.Contains(cleanName, "-db") || strings.Contains(cleanName, "_db") || strings.EqualFold(cleanName, "db") {
			for _, line := range item.Config.Env {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					k, v := parts[0], parts[1]
					if k == "POSTGRES_PASSWORD" {
						env["DB_PASSWORD"] = v
					} else if k == "POSTGRES_USER" {
						env["DB_USER"] = v
					} else if k == "POSTGRES_DB" {
						env["DB_NAME"] = v
					}
				}
			}
		}
		if strings.Contains(cleanName, "crm-server") || strings.Contains(cleanName, "-server") || strings.Contains(cleanName, "_server") || strings.EqualFold(cleanName, "server") {
			for _, line := range item.Config.Env {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					k, v := parts[0], parts[1]
					switch k {
					case "DATABASE_URL":
						env["DATABASE_URL"] = v
					case "INITIAL_ADMIN_EMAIL":
						env["INITIAL_ADMIN_EMAIL"] = v
					case "INITIAL_ADMIN_PASSWORD":
						env["INITIAL_ADMIN_PASSWORD"] = v
					case "CONNECTOR_API_TOKEN":
						env["CONNECTOR_API_TOKEN"] = v
					case "AI_PROVIDER":
						env["AI_PROVIDER"] = v
					case "OLLAMA_BASE_URL":
						env["OLLAMA_BASE_URL"] = v
					case "OLLAMA_MODEL":
						env["OLLAMA_MODEL"] = v
					case "AI_API_KEY":
						env["AI_API_KEY"] = v
					case "AI_BASE_URL":
						env["AI_BASE_URL"] = v
					case "AI_EMBEDDING_MODEL":
						env["AI_EMBEDDING_MODEL"] = v
					case "OLLAMA_EMBEDDING_MODEL":
						env["OLLAMA_EMBEDDING_MODEL"] = v
					case "DEMO_MODE":
						env["DEMO_MODE"] = v
					}
				}
			}
		}
		if strings.Contains(cleanName, "crm-proxy") || strings.Contains(cleanName, "caddy") || strings.Contains(cleanName, "proxy") {
			for portKey, bindings := range item.NetworkSettings.Ports {
				if strings.HasPrefix(portKey, "80/") && len(bindings) > 0 && bindings[0].HostPort != "" {
					env["APP_PORT"] = bindings[0].HostPort
					env["PORT"] = bindings[0].HostPort
				}
			}
		}
	}
	return env
}

func ConvertInspectToContainerInfo(items []DockerInspectItem) []ContainerInfo {
	var list []ContainerInfo
	for _, item := range items {
		cleanName := strings.TrimPrefix(item.Name, "/")
		service := item.Config.Labels["com.docker.compose.service"]
		if service == "" {
			switch {
			case strings.Contains(cleanName, "proxy"):
				service = "caddy"
			case strings.Contains(cleanName, "server"):
				service = "server"
			case strings.Contains(cleanName, "worker"):
				service = "worker"
			case strings.Contains(cleanName, "db"):
				service = "db"
			default:
				service = cleanName
			}
		}

		info := ContainerInfo{
			Name:    cleanName,
			Service: service,
			State:   item.State.Status,
			Status:  item.State.Status,
			Health:  item.State.Health.Status,
		}
		list = append(list, info)
	}
	return list
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func ParseContainersJSON(data []byte) ([]ContainerInfo, error) {
	var list []ContainerInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

type Engine struct {
	BaseDir string
}

func NewEngine(baseDir string) *Engine {
	return &Engine{BaseDir: baseDir}
}

func (e *Engine) getDBUserAndName() (string, string) {
	env := ReadExistingEnvMap(e.BaseDir)
	user := env["DB_USER"]
	if user == "" {
		user = "postgres"
	}
	dbName := env["DB_NAME"]
	if dbName == "" {
		if dbURL := env["DATABASE_URL"]; dbURL != "" {
			if idx := strings.LastIndex(dbURL, "/"); idx != -1 {
				rest := dbURL[idx+1:]
				if qIdx := strings.Index(rest, "?"); qIdx != -1 {
					rest = rest[:qIdx]
				}
				if rest != "" {
					dbName = rest
				}
			}
		}
	}
	if dbName == "" {
		dbName = "openlocalcrm"
	}
	return user, dbName
}

func (e *Engine) Up(ctx context.Context, logChan chan<- string) error {
	cmd := execCommand(ctx, "docker", "compose", "up", "-d", "--remove-orphans")
	cmd.Dir = e.BaseDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if logChan != nil {
			logChan <- line
		}
	}

	return cmd.Wait()
}

func (e *Engine) Stop(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "stop")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

func (e *Engine) Restart(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "restart")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

func (e *Engine) Down(ctx context.Context) error {
	cmd := execCommand(ctx, "docker", "compose", "down")
	cmd.Dir = e.BaseDir
	return cmd.Run()
}

// ResetAll stops all containers, removes all volumes and orphans, deletes lingering crm containers/volumes,
// and deletes the local .env configuration so the system can be cleanly re-installed from scratch.
func (e *Engine) ResetAll(ctx context.Context, logChan chan<- string) error {
	if logChan != nil {
		logChan <- "[Reset] Starte vollständigen Factory Reset..."
	}

	// 1. Docker compose down -v --remove-orphans
	if logChan != nil {
		logChan <- "[Reset] Stoppe und entferne Docker-Container & Volumes (docker compose down -v)..."
	}
	downCmd := execCommand(ctx, "docker", "compose", "down", "-v", "--remove-orphans")
	downCmd.Dir = e.BaseDir
	if out, err := downCmd.CombinedOutput(); err != nil {
		if logChan != nil {
			logChan <- fmt.Sprintf("[Reset Hinweis] compose down: %s (%v)", strings.TrimSpace(string(out)), err)
		}
	} else if logChan != nil && len(out) > 0 {
		logChan <- fmt.Sprintf("[Reset] %s", strings.TrimSpace(string(out)))
	}

	// 2. Force remove lingering CRM containers if any (including legacy mavalio)
	if logChan != nil {
		logChan <- "[Reset] Bereinige verbleibende CRM-Container..."
	}
	lingeringContainers := []string{
		"crm-db", "crm-server", "crm-proxy", "crm-worker",
		"openlocalcrm-db-1", "openlocalcrm-server-1", "openlocalcrm-caddy-1", "openlocalcrm-worker-1",
		"mavalio-db-1", "mavalio-server-1", "mavalio-caddy-1", "mavalio-worker-1",
	}
	for _, name := range lingeringContainers {
		rmCmd := execCommand(ctx, "docker", "rm", "-f", name)
		_ = rmCmd.Run()
	}
	// Dynamic container cleanup for any remaining matches
	if psOut, psErr := execCommand(ctx, "docker", "ps", "-a", "--format", "{{.Names}}").Output(); psErr == nil {
		for _, line := range strings.Split(string(psOut), "\n") {
			name := strings.TrimSpace(line)
			cleanName := strings.TrimPrefix(name, "/")
			lower := strings.ToLower(cleanName)
			if lower != "" && (strings.Contains(lower, "crm") || strings.Contains(lower, "openlocalcrm") || strings.Contains(lower, "mavalio")) {
				_ = execCommand(ctx, "docker", "rm", "-f", cleanName).Run()
			}
		}
	}

	// 3. Force remove persistent CRM volumes (including legacy mavalio)
	if logChan != nil {
		logChan <- "[Reset] Bereinige persistente Docker-Volumes..."
	}
	lingeringVolumes := []string{
		"openlocalcrm_pg_data", "crm_pg_data", "mavalio_pg_data",
		"openlocalcrm_crm_storage", "crm_storage", "mavalio_crm_storage",
		"openlocalcrm_crm_keys", "crm_keys", "mavalio_crm_keys",
		"openlocalcrm_crm_caddy_data", "crm_caddy_data", "mavalio_crm_caddy_data",
		"openlocalcrm_crm_caddy_config", "crm_caddy_config", "mavalio_crm_caddy_config",
	}
	for _, vol := range lingeringVolumes {
		volCmd := execCommand(ctx, "docker", "volume", "rm", "-f", vol)
		_ = volCmd.Run()
	}
	// Dynamic volume cleanup for any remaining matches
	if volOut, volErr := execCommand(ctx, "docker", "volume", "ls", "--format", "{{.Name}}").Output(); volErr == nil {
		for _, line := range strings.Split(string(volOut), "\n") {
			vol := strings.TrimSpace(line)
			lower := strings.ToLower(vol)
			if lower != "" && (strings.Contains(lower, "crm") || strings.Contains(lower, "openlocalcrm") || strings.Contains(lower, "mavalio")) {
				_ = execCommand(ctx, "docker", "volume", "rm", "-f", vol).Run()
			}
		}
	}

	// 4. Delete local .env file
	envFile := filepath.Join(e.BaseDir, ".env")
	if err := os.Remove(envFile); err != nil && !os.IsNotExist(err) {
		if logChan != nil {
			logChan <- fmt.Sprintf("[Reset Warnung] .env konnte nicht gelöscht werden: %v", err)
		}
	} else if logChan != nil {
		logChan <- "[Reset] Lokale Konfiguration (.env) entfernt."
	}

	if logChan != nil {
		logChan <- "[Reset] ✅ Factory Reset erfolgreich abgeschlossen. Das System kann nun sauber neu aufgesetzt werden."
	}
	return nil
}

func (e *Engine) RecoverEnvFromDocker(ctx context.Context) (map[string]string, error) {
	candidates := []string{"crm-db", "crm-server", "crm-proxy", "crm-worker", "openlocalcrm-db-1", "openlocalcrm-server-1", "openlocalcrm-caddy-1"}

	psCmd := execCommand(ctx, "docker", "ps", "-a", "--format", "{{.Names}}")
	if psOut, err := psCmd.Output(); err == nil {
		lines := strings.Split(string(psOut), "\n")
		for _, line := range lines {
			name := strings.TrimSpace(line)
			if name != "" && (strings.Contains(name, "crm") || strings.Contains(name, "openlocalcrm") || strings.Contains(name, "mavalio")) {
				candidates = append(candidates, name)
			}
		}
	}

	seen := make(map[string]bool)
	var allItems []DockerInspectItem

	for _, name := range candidates {
		if seen[name] {
			continue
		}
		seen[name] = true

		inspectCmd := execCommand(ctx, "docker", "inspect", name)
		if out, err := inspectCmd.Output(); err == nil {
			if items, pErr := ParseDockerInspectJSON(out); pErr == nil && len(items) > 0 {
				allItems = append(allItems, items...)
			}
		}
	}

	if len(allItems) == 0 {
		return nil, fmt.Errorf("no openlocalcrm containers found in docker")
	}

	env := ExtractEnvFromInspect(allItems)
	if len(env) == 0 {
		return nil, fmt.Errorf("no environment variables found in inspected containers")
	}
	return env, nil
}

func (e *Engine) GetContainers(ctx context.Context) ([]ContainerInfo, error) {
	cmd := execCommand(ctx, "docker", "compose", "ps", "--format", "json")
	cmd.Dir = e.BaseDir

	out, err := cmd.Output()
	if err == nil {
		if list, parseErr := ParseContainersJSON(out); parseErr == nil && len(list) > 0 {
			return list, nil
		}
	}

	// Direct query against Docker daemon via docker ps -a (works across different project/folder names)
	psCmd := execCommand(ctx, "docker", "ps", "-a", "--format", "{{json .}}")
	if psOut, psErr := psCmd.Output(); psErr == nil && len(psOut) > 0 {
		var list []ContainerInfo
		lines := strings.Split(string(psOut), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var item struct {
				Names  string `json:"Names"`
				State  string `json:"State"`
				Status string `json:"Status"`
			}
			if err := json.Unmarshal([]byte(line), &item); err == nil {
				cleanName := strings.TrimPrefix(item.Names, "/")
				if strings.Contains(cleanName, "crm") || strings.Contains(cleanName, "openlocalcrm") || strings.Contains(cleanName, "mavalio") {
					service := cleanName
					switch {
					case strings.Contains(cleanName, "proxy") || strings.Contains(cleanName, "caddy"):
						service = "caddy"
					case strings.Contains(cleanName, "server"):
						service = "server"
					case strings.Contains(cleanName, "worker"):
						service = "worker"
					case strings.Contains(cleanName, "db") || strings.Contains(cleanName, "postgres"):
						service = "db"
					}
					list = append(list, ContainerInfo{
						Name:    cleanName,
						Service: service,
						State:   item.State,
						Status:  item.Status,
						Health:  item.Status,
					})
				}
			}
		}
		if len(list) > 0 {
			return list, nil
		}
	}

	// Fallback to inspecting individual known containers
	candidateNames := []string{"crm-proxy", "crm-server", "crm-worker", "crm-db"}
	var inspectItems []DockerInspectItem
	for _, name := range candidateNames {
		inspectCmd := execCommand(ctx, "docker", "inspect", name)
		if inspectOut, inspectErr := inspectCmd.Output(); inspectErr == nil {
			if items, pErr := ParseDockerInspectJSON(inspectOut); pErr == nil && len(items) > 0 {
				inspectItems = append(inspectItems, items...)
			}
		}
	}
	if len(inspectItems) > 0 {
		return ConvertInspectToContainerInfo(inspectItems), nil
	}

	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no containers found")
}

func GetPersistentBackupDir() string {
	if custom := os.Getenv("OPENLOCALCRM_BACKUP_DIR"); custom != "" {
		_ = os.MkdirAll(custom, 0755)
		return custom
	}
	var dir string
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			dir = filepath.Join(localAppData, "openlocalcrm", "backups")
		}
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, ".openlocalcrm", "backups")
		} else {
			dir = filepath.Join(os.TempDir(), "openlocalcrm-backups")
		}
	}
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (e *Engine) InspectBackupFile(filename string) BackupInfo {
	cleanName := filepath.Base(filename)
	backupPath := filepath.Join(e.BaseDir, "backups", cleanName)

	info := BackupInfo{Filename: cleanName}
	stat, err := os.Stat(backupPath)
	if err == nil {
		info.SizeBytes = stat.Size()
		info.SizeFormatted = formatBytes(stat.Size())
		info.CreatedAt = stat.ModTime()
	}

	file, err := os.Open(backupPath)
	if err != nil {
		return info
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	linesRead := 0
	schemaRegex := regexp.MustCompile(`--\s*SCHEMA_VERSION:\s*([^\s]+)`)
	insertRegex := regexp.MustCompile(`schema_migrations.*VALUES\s*\('([^']+)'\)`)

	var latestVersion string
	for scanner.Scan() && linesRead < 200 {
		line := scanner.Text()
		linesRead++

		if matches := schemaRegex.FindStringSubmatch(line); len(matches) > 1 {
			info.SchemaVersion = matches[1]
			return info
		}

		if matches := insertRegex.FindStringSubmatch(line); len(matches) > 1 {
			v := matches[1]
			if v > latestVersion {
				latestVersion = v
			}
		}
	}

	if latestVersion != "" {
		info.SchemaVersion = latestVersion
	}
	return info
}

func (e *Engine) CreateBackup(ctx context.Context) (string, error) {
	backupDir := filepath.Join(e.BaseDir, "backups")
	_ = os.MkdirAll(backupDir, 0755)

	filename := fmt.Sprintf("openlocalcrm_backup_%s.sql", time.Now().Format("2006-01-02_150405"))
	outPath := filepath.Join(backupDir, filename)

	file, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	header := fmt.Sprintf("-- ==============================================================================\n"+
		"-- OpenLocalCRM Database Backup\n"+
		"-- CREATED_AT: %s\n"+
		"-- ==============================================================================\n\n",
		time.Now().UTC().Format(time.RFC3339))
	_, _ = file.WriteString(header)

	user, dbName := e.getDBUserAndName()
	cmd := execCommand(ctx, "docker", "compose", "exec", "-T", "db", "pg_dump", "-U", user, "--clean", "--if-exists", dbName)
	cmd.Dir = e.BaseDir
	cmd.Stdout = file

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("pg_dump failed: %w", err)
	}

	// Mirror backup to persistent directory so it survives directory deletion during updates
	persistentDir := GetPersistentBackupDir()
	persistentPath := filepath.Join(persistentDir, filename)
	if data, readErr := os.ReadFile(outPath); readErr == nil {
		_ = os.WriteFile(persistentPath, data, 0644)
	}

	return filename, nil
}

func (e *Engine) ListBackups() ([]BackupInfo, error) {
	backupDir := filepath.Join(e.BaseDir, "backups")
	_ = os.MkdirAll(backupDir, 0755)

	persistentDirs := []string{GetPersistentBackupDir()}
	// Legacy persistent backup fallback: if mavalio backups exist, sync them too!
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			legacyDir := filepath.Join(localAppData, "mavalio", "backups")
			if _, err := os.Stat(legacyDir); err == nil {
				persistentDirs = append(persistentDirs, legacyDir)
			}
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		legacyDir := filepath.Join(home, ".mavalio", "backups")
		if _, err := os.Stat(legacyDir); err == nil {
			persistentDirs = append(persistentDirs, legacyDir)
		}
	}

	// If any backups exist in persistentDirs but not in baseDir/backups (e.g. after folder replace), copy them over
	for _, pDir := range persistentDirs {
		if pEntries, err := os.ReadDir(pDir); err == nil {
			for _, entry := range pEntries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
					target := filepath.Join(backupDir, entry.Name())
					if _, err := os.Stat(target); os.IsNotExist(err) {
						if data, err := os.ReadFile(filepath.Join(pDir, entry.Name())); err == nil {
							_ = os.WriteFile(target, data, 0644)
						}
					}
				}
			}
		}
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var list []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if seen[entry.Name()] {
			continue
		}
		seen[entry.Name()] = true
		info := e.InspectBackupFile(entry.Name())
		list = append(list, info)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list, nil
}

func (e *Engine) RestoreBackup(ctx context.Context, filename string, logChan chan<- string) error {
	cleanName := filepath.Base(filename)
	if cleanName != filename || cleanName == "." || cleanName == ".." || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("invalid backup filename: directory traversal detected")
	}

	backupPath := filepath.Join(e.BaseDir, "backups", cleanName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		persistentPath := filepath.Join(GetPersistentBackupDir(), cleanName)
		if data, err := os.ReadFile(persistentPath); err == nil {
			_ = os.MkdirAll(filepath.Join(e.BaseDir, "backups"), 0755)
			_ = os.WriteFile(backupPath, data, 0644)
		} else if runtime.GOOS == "windows" {
			if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
				if legacyData, lErr := os.ReadFile(filepath.Join(localAppData, "mavalio", "backups", cleanName)); lErr == nil {
					_ = os.MkdirAll(filepath.Join(e.BaseDir, "backups"), 0755)
					_ = os.WriteFile(backupPath, legacyData, 0644)
				}
			}
		}
	}

	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed opening backup file: %w", err)
	}
	defer file.Close()

	meta := e.InspectBackupFile(cleanName)
	if logChan != nil {
		if meta.SchemaVersion != "" {
			logChan <- fmt.Sprintf("[RESTORE] Starte Wiederherstellung von '%s' (gespeichertes Schema: %s)...", cleanName, meta.SchemaVersion)
		} else {
			logChan <- fmt.Sprintf("[RESTORE] Starte Wiederherstellung von '%s'...", cleanName)
		}
	}

	// 1. Create safety snapshot before restoring
	if logChan != nil {
		logChan <- "[RESTORE] Schritt 1/5: Erstelle automatischen Sicherheits-Snapshot der aktuellen Datenbank..."
	}
	if snap, err := e.CreateBackup(ctx); err == nil && logChan != nil {
		logChan <- fmt.Sprintf("[RESTORE] Sicherheits-Snapshot gesichert als: %s", snap)
	}

	// 2. Stop active application containers to prevent concurrent transactions or connection locks
	if logChan != nil {
		logChan <- "[RESTORE] Schritt 2/5: Halte Server- und Worker-Container temporär an..."
	}
	stopCmd := execCommand(ctx, "docker", "compose", "stop", "server", "worker")
	stopCmd.Dir = e.BaseDir
	_ = stopCmd.Run()

	user, dbName := e.getDBUserAndName()

	// 3. Clean target database schema to allow clean import without "already exists" conflicts
	if logChan != nil {
		logChan <- "[RESTORE] Schritt 3/5: Bereinige Datenbank-Schema für sauberen DDL-Import..."
	}
	cleanSQL := fmt.Sprintf("DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO %s; GRANT ALL ON SCHEMA public TO public; CREATE EXTENSION IF NOT EXISTS vector;", user)
	cleanCmd := execCommand(ctx, "docker", "compose", "exec", "-T", "db", "psql", "-U", user, "-d", dbName, "-c", cleanSQL)
	cleanCmd.Dir = e.BaseDir
	if cleanErr := cleanCmd.Run(); cleanErr != nil && logChan != nil {
		logChan <- fmt.Sprintf("[WARNUNG] Schema-Bereinigung meldete: %v (fahre mit Import fort)", cleanErr)
	}

	// 4. Pipe SQL dump into psql
	if logChan != nil {
		logChan <- fmt.Sprintf("[RESTORE] Schritt 4/5: Spiele SQL-Dump '%s' in PostgreSQL ein...", cleanName)
	}

	cmd := execCommand(ctx, "docker", "compose", "exec", "-T", "db", "psql", "-U", user, "-d", dbName)
	cmd.Dir = e.BaseDir
	cmd.Stdin = file

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed starting psql restore: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		text := scanner.Text()
		if logChan != nil {
			logChan <- text
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("restore command failed: %w", err)
	}

	// 5. Restart application containers to automatically run forward-migrations on the restored database
	if logChan != nil {
		logChan <- "[RESTORE] Schritt 5/5: Starte Server & Worker neu — automatische Schema-Migrationen werden ausgeführt..."
	}
	startCmd := execCommand(ctx, "docker", "compose", "up", "-d", "server", "worker")
	startCmd.Dir = e.BaseDir
	_ = startCmd.Run()

	if logChan != nil {
		logChan <- "[RESTORE] ✅ Wiederherstellung erfolgreich abgeschlossen! Datenbank ist nun betriebsbereit und auf aktuellem Schema-Stand."
	}
	return nil
}

func (e *Engine) Update(ctx context.Context, logChan chan<- string) error {
	if logChan != nil {
		logChan <- "[UPDATE] Initiating container rebuild and update..."
		logChan <- "[UPDATE] Step 1/2: Creating automatic safety backup..."
	}

	snap, err := e.CreateBackup(ctx)
	if err != nil {
		if logChan != nil {
			logChan <- fmt.Sprintf("[WARNING] Pre-update backup failed: %v. Proceeding...", err)
		}
	} else if logChan != nil {
		logChan <- fmt.Sprintf("[UPDATE] Safety backup saved to %s", snap)
	}

	if logChan != nil {
		logChan <- "[UPDATE] Step 2/2: Running docker compose up -d --build --remove-orphans..."
	}

	cmd := execCommand(ctx, "docker", "compose", "up", "-d", "--build", "--remove-orphans")
	cmd.Dir = e.BaseDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start container update: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		text := scanner.Text()
		if logChan != nil {
			logChan <- text
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("container update failed: %w", err)
	}

	if logChan != nil {
		logChan <- "[UPDATE] Containers updated and rebuilt successfully!"
	}
	return nil
}

// ResetAdminPassword resets or bootstraps the administrator's password directly in PostgreSQL
func (e *Engine) ResetAdminPassword(ctx context.Context, email, newPassword string) error {
	cleanEmail := strings.TrimSpace(strings.ToLower(email))
	if cleanEmail == "" {
		return fmt.Errorf("E-Mail-Adresse darf nicht leer sein")
	}
	if len(newPassword) < 6 {
		return fmt.Errorf("Passwort muss mindestens 6 Zeichen lang sein")
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("Passwort-Hashing fehlgeschlagen: %w", err)
	}

	user, dbName := e.getDBUserAndName()

	// 1. Ensure db container is running
	startCmd := execCommand(ctx, "docker", "compose", "up", "-d", "db")
	startCmd.Dir = e.BaseDir
	_ = startCmd.Run()

	// Wait up to 5 seconds for database readiness
	for i := 0; i < 10; i++ {
		checkCmd := execCommand(ctx, "docker", "compose", "exec", "-T", "db", "pg_isready", "-U", user, "-d", dbName)
		checkCmd.Dir = e.BaseDir
		if err := checkCmd.Run(); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	// 2. Execute SQL query to update user or insert if not existing yet
	escapedHash := strings.ReplaceAll(hash, "'", "''")
	escapedEmail := strings.ReplaceAll(cleanEmail, "'", "''")

	sql := fmt.Sprintf(`
DO $$
BEGIN
    IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users') THEN
        UPDATE users 
        SET password_hash = '%s',
            role = 'ADMIN',
            status = 'ACTIVE',
            totp_enabled = FALSE,
            totp_secret_encrypted = NULL,
            updated_at = NOW()
        WHERE LOWER(email) = LOWER('%s');

        IF NOT FOUND THEN
            INSERT INTO users (id, email, password_hash, first_name, last_name, role, status)
            VALUES (gen_random_uuid(), LOWER('%s'), '%s', 'Admin', 'ADMIN', 'ADMIN', 'ACTIVE');
        END IF;

        IF EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'refresh_tokens') THEN
            DELETE FROM refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = LOWER('%s'));
        END IF;
    END IF;
END $$;
`, escapedHash, escapedEmail, escapedEmail, escapedHash, escapedEmail)

	cmd := execCommand(ctx, "docker", "compose", "exec", "-T", "db", "psql", "-U", user, "-d", dbName)
	cmd.Dir = e.BaseDir
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Datenbank-Aktualisierung fehlgeschlagen: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// 3. Keep .env in sync
	envMap := ReadExistingEnvMap(e.BaseDir)
	envMap["INITIAL_ADMIN_EMAIL"] = cleanEmail
	envMap["INITIAL_ADMIN_PASSWORD"] = newPassword
	cfg, hasCfg := ReadExistingConfig(e.BaseDir)
	if hasCfg && cfg != nil {
		cfg.AdminEmail = cleanEmail
		cfg.AdminPassword = newPassword
		_ = WriteConfigAndDirectoriesWithEnv(e.BaseDir, *cfg, envMap)
	}

	return nil
}
