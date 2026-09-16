package launcher

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	BuildCommit = "dev"
	BuildDate   = ""
)

var protectedPaths = []string{
	".env",
	".env.local",
	".env.production",
	"backups",
	"data",
	"storage",
	".git",
	"openlocalcrm-setup.log",
	"caddy/certs",
}

type UpdateCheckResult struct {
	HasUpdate     bool      `json:"has_update"`
	CurrentCommit string    `json:"current_commit"`
	LatestCommit  string    `json:"latest_commit"`
	CommitMessage string    `json:"commit_message"`
	CommitDate    string    `json:"commit_date"`
	CheckedAt     time.Time `json:"checked_at"`
	RateLimited   bool      `json:"rate_limited"`
	Error         string    `json:"error,omitempty"`
}

type cachedCheck struct {
	etag      string
	result    *UpdateCheckResult
	timestamp time.Time
}

type Updater struct {
	BaseDir string
	Client  *http.Client
	RepoURL string

	cacheMu sync.RWMutex
	cache   *cachedCheck
}

func NewUpdater(baseDir string) *Updater {
	return &Updater{
		BaseDir: baseDir,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
		RepoURL: "https://api.github.com/repos/boreddev1/openlocalcrm/commits/main",
	}
}

func (u *Updater) CheckForUpdate(currentCommit string) (*UpdateCheckResult, error) {
	cleanCurrent := strings.TrimSpace(currentCommit)
	if cleanCurrent == "" {
		cleanCurrent = BuildCommit
	}

	u.cacheMu.RLock()
	if u.cache != nil && time.Since(u.cache.timestamp) < 15*time.Minute && u.cache.result != nil {
		res := *u.cache.result
		u.cacheMu.RUnlock()
		res.CurrentCommit = cleanCurrent
		res.HasUpdate = isNewerCommit(cleanCurrent, res.LatestCommit)
		return &res, nil
	}
	u.cacheMu.RUnlock()

	req, err := http.NewRequest(http.MethodGet, u.RepoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update check request: %w", err)
	}

	req.Header.Set("User-Agent", "OpenLocalCRM-Updater/3.0 (+https://github.com/boreddev1/openlocalcrm)")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	u.cacheMu.RLock()
	if u.cache != nil && u.cache.etag != "" {
		req.Header.Set("If-None-Match", u.cache.etag)
	}
	u.cacheMu.RUnlock()

	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		u.cacheMu.Lock()
		if u.cache != nil && u.cache.result != nil {
			u.cache.timestamp = time.Now()
			res := *u.cache.result
			u.cacheMu.Unlock()
			res.CurrentCommit = cleanCurrent
			res.HasUpdate = isNewerCommit(cleanCurrent, res.LatestCommit)
			return &res, nil
		}
		u.cacheMu.Unlock()
	}

	if resp.StatusCode == http.StatusForbidden {
		return &UpdateCheckResult{
			HasUpdate:     false,
			CurrentCommit: cleanCurrent,
			RateLimited:   true,
			CheckedAt:     time.Now(),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var ghCommit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghCommit); err != nil {
		return nil, fmt.Errorf("failed parsing github commit json: %w", err)
	}

	res := &UpdateCheckResult{
		HasUpdate:     isNewerCommit(cleanCurrent, ghCommit.SHA),
		CurrentCommit: cleanCurrent,
		LatestCommit:  ghCommit.SHA,
		CommitMessage: ghCommit.Commit.Message,
		CommitDate:    ghCommit.Commit.Committer.Date,
		CheckedAt:     time.Now(),
		RateLimited:   false,
	}

	u.cacheMu.Lock()
	u.cache = &cachedCheck{
		etag:      resp.Header.Get("ETag"),
		result:    res,
		timestamp: time.Now(),
	}
	u.cacheMu.Unlock()

	return res, nil
}

func isNewerCommit(current, latest string) bool {
	c := strings.TrimSpace(current)
	l := strings.TrimSpace(latest)
	if l == "" {
		return false
	}
	if c == "" || c == "dev" {
		return false
	}
	if strings.HasPrefix(l, c) || strings.HasPrefix(c, l) {
		return false
	}
	return true
}

func (u *Updater) IsProtectedPath(relPath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relPath))
	if clean == "." || clean == "/" {
		return false
	}
	if strings.HasPrefix(clean, "./") {
		clean = strings.TrimPrefix(clean, "./")
	}

	if strings.HasSuffix(clean, ".old") {
		return true
	}

	for _, p := range protectedPaths {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}

func (u *Updater) ExtractZipSafely(zipReader *zip.Reader, destDir string) error {
	cleanDest := filepath.Clean(destDir)

	for _, file := range zipReader.File {
		// Defense-in-depth: Immediately reject any entry containing ".." or absolute path root
		if strings.Contains(file.Name, "..") || strings.HasPrefix(file.Name, "/") || strings.HasPrefix(file.Name, "\\") {
			return fmt.Errorf("illegal path traversal detected in zip: %s", file.Name)
		}

		parts := strings.Split(filepath.ToSlash(file.Name), "/")
		if len(parts) <= 1 {
			continue // skip root container dir itself
		}

		relPath := strings.Join(parts[1:], "/")
		if strings.TrimSpace(relPath) == "" {
			continue
		}

		targetPath := filepath.Clean(filepath.Join(cleanDest, filepath.FromSlash(relPath)))

		// Zip Slip Guard
		if !strings.HasPrefix(targetPath, cleanDest+string(os.PathSeparator)) && targetPath != cleanDest {
			return fmt.Errorf("illegal path traversal detected in zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (u *Updater) ApplyStagedFiles(stagingDir string, logChan chan<- string) error {
	cleanBase := filepath.Clean(u.BaseDir)
	cleanStaging := filepath.Clean(stagingDir)

	var envBackedUp bool

	err := filepath.Walk(cleanStaging, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(cleanStaging, path)
		if err != nil || relPath == "." || relPath == "" {
			return nil
		}

		// Don't overwrite protected files (like .env, backups/, .git/, etc.)
		if u.IsProtectedPath(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			if logChan != nil {
				logChan <- fmt.Sprintf("[Update] Geschützter Pfad wird beibehalten: %s", relPath)
			}
			return nil
		}

		// Skip all binaries in bin/ during normal file copy; HotSwapExecutable handles them
		slashRel := filepath.ToSlash(relPath)
		if slashRel == "bin" || strings.HasPrefix(slashRel, "bin/") {
			if info.IsDir() {
				// Don't skip dir because bin/ might contain subdirs, but skip copying binary files here
				return nil
			}
			return nil
		}

		targetPath := filepath.Join(cleanBase, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// Backup .env once if it exists in baseDir
		if !envBackedUp {
			envPath := filepath.Join(cleanBase, ".env")
			if _, statErr := os.Stat(envPath); statErr == nil {
				bakName := fmt.Sprintf(".env.backup.%s", time.Now().Format("20060102_150405"))
				bakPath := filepath.Join(cleanBase, bakName)
				if copyErr := copyFile(envPath, bakPath, 0600); copyErr == nil && logChan != nil {
					logChan <- fmt.Sprintf("[Update] Sicherheitskopie der Konfiguration erstellt: %s", bakName)
				}
			}
			envBackedUp = true
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		return copyFile(path, targetPath, info.Mode())
	})

	return err
}

func (u *Updater) HotSwapExecutable(stagingDir string, logChan chan<- string) (string, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine current executable: %w", err)
	}

	exeName := filepath.Base(currentExe)
	targetPlatformName := fmt.Sprintf("openlocalcrm-setup-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		targetPlatformName += ".exe"
	}

	candidates := []string{
		filepath.Join(stagingDir, "bin", targetPlatformName),
		filepath.Join(stagingDir, "bin", exeName),
		filepath.Join(stagingDir, "bin", "openlocalcrm"),
		filepath.Join(stagingDir, "bin", "openlocalcrm-setup.exe"),
		filepath.Join(stagingDir, exeName),
	}

	if strings.Contains(strings.ToLower(exeName), "debug") {
		candidates = append([]string{filepath.Join(stagingDir, "bin", "openlocalcrm-setup-debug.exe")}, candidates...)
	}

	var sourceBinary string
	for _, c := range candidates {
		if fi, statErr := os.Stat(c); statErr == nil && !fi.IsDir() {
			sourceBinary = c
			break
		}
	}

	if sourceBinary == "" {
		return "", fmt.Errorf("no replacement binary found in staging for %s (searched: %v)", exeName, candidates)
	}

	if logChan != nil {
		logChan <- fmt.Sprintf("[Update] Ersetze Executable %s durch neue Version (%s)...", exeName, filepath.Base(sourceBinary))
	}

	// Unix systems allow atomic inode unlinking & replacement even for executing binaries
	if runtime.GOOS != "windows" {
		tmpExe := currentExe + ".update_tmp"
		_ = os.Remove(tmpExe)

		if err := copyFile(sourceBinary, tmpExe, 0755); err != nil {
			if os.IsPermission(err) {
				return "", fmt.Errorf("keine Schreibberechtigung für %s (bitte mit 'sudo openlocalcrm update' ausführen): %w", currentExe, err)
			}
			return "", fmt.Errorf("failed staging binary swap: %w", err)
		}
		if err := os.Chmod(tmpExe, 0755); err != nil {
			_ = os.Remove(tmpExe)
			return "", err
		}
		if err := os.Rename(tmpExe, currentExe); err != nil {
			_ = os.Remove(tmpExe)
			if os.IsPermission(err) {
				return "", fmt.Errorf("keine Schreibberechtigung für %s (bitte mit 'sudo openlocalcrm update' ausführen): %w", currentExe, err)
			}
			return "", fmt.Errorf("failed atomic executable rename: %w", err)
		}

		if logChan != nil {
			logChan <- "[Update] Neue Binärdatei erfolgreich atomar ausgetauscht!"
		}
		return currentExe, nil
	}

	// Windows file-locking requires renaming current .exe to .old first
	oldExe := currentExe + ".old"
	_ = os.Remove(oldExe)

	var renameErr error
	for attempt := 1; attempt <= 5; attempt++ {
		renameErr = os.Rename(currentExe, oldExe)
		if renameErr == nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if renameErr != nil {
		return "", fmt.Errorf("failed renaming current executable to %s: %w", oldExe, renameErr)
	}

	copyErr := copyFile(sourceBinary, currentExe, 0755)
	if copyErr != nil {
		// Rollback attempt
		_ = os.Rename(oldExe, currentExe)
		return "", fmt.Errorf("failed copying new binary into place: %w", copyErr)
	}

	if logChan != nil {
		logChan <- "[Update] Neue Binärdatei erfolgreich platziert!"
	}

	return currentExe, nil
}

func (u *Updater) CleanupStaleOldExecutablesInDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".old") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (u *Updater) CleanupStaleOldExecutables() {
	if exePath, err := os.Executable(); err == nil {
		u.CleanupStaleOldExecutablesInDir(filepath.Dir(exePath))
	}
	u.CleanupStaleOldExecutablesInDir(u.BaseDir)
	binDir := filepath.Join(u.BaseDir, "bin")
	u.CleanupStaleOldExecutablesInDir(binDir)
}

func (u *Updater) ExecuteUpdate(ctx context.Context, engine *Engine, logChan chan<- string) error {
	if logChan != nil {
		logChan <- "[Update] Starte Aktualisierung von GitHub..."
		logChan <- "[Update] Schritt 1/6: Erstelle Sicherheits-Backup der Datenbank..."
	}

	if engine != nil {
		snap, err := engine.CreateBackup(ctx)
		if err != nil {
			if logChan != nil {
				logChan <- fmt.Sprintf("[Warnung] Pre-Update DB-Backup fehlgeschlagen: %v (fahre fort...)", err)
			}
		} else if logChan != nil {
			logChan <- fmt.Sprintf("[Update] Automatisches Sicherheits-Backup erstellt: %s", snap)
		}
	}

	if logChan != nil {
		logChan <- "[Update] Schritt 2/6: Lade neuestes Repository-Archiv von GitHub herunter..."
	}

	zipURL := "https://github.com/boreddev1/openlocalcrm/archive/refs/heads/main.zip"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", "OpenLocalCRM-Updater/3.0 (+https://github.com/boreddev1/openlocalcrm)")

	resp, err := u.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed downloading repository archive from github: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading repository archive failed with status %d", resp.StatusCode)
	}

	tmpZip, err := os.CreateTemp("", "openlocalcrm_update_*.zip")
	if err != nil {
		return fmt.Errorf("failed creating temp file for update zip: %w", err)
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath)

	zipSize, err := io.Copy(tmpZip, resp.Body)
	_ = tmpZip.Close()
	if err != nil {
		return fmt.Errorf("failed saving repository zip: %w", err)
	}

	if logChan != nil {
		logChan <- fmt.Sprintf("[Update] Archiv heruntergeladen (%s).", formatBytes(zipSize))
		logChan <- "[Update] Schritt 3/6: Entpacke Dateien im Quarantäne-Staging..."
	}

	stagingDir := filepath.Join(u.BaseDir, ".openlocalcrm_update_staging")
	_ = os.RemoveAll(stagingDir)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("failed creating staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	zr, err := zip.OpenReader(tmpZipPath)
	if err != nil {
		return fmt.Errorf("failed opening update zip archive: %w", err)
	}
	defer zr.Close()

	if err := u.ExtractZipSafely(&zr.Reader, stagingDir); err != nil {
		return fmt.Errorf("failed extracting update zip archive: %w", err)
	}

	if logChan != nil {
		logChan <- "[Update] Schritt 4/6: Aktualisiere System- und Repository-Dateien..."
	}

	if err := u.ApplyStagedFiles(stagingDir, logChan); err != nil {
		return fmt.Errorf("failed applying staged files: %w", err)
	}

	if logChan != nil {
		logChan <- "[Update] Schritt 5/6: Aktualisiere Binärdatei..."
	}

	_, swapErr := u.HotSwapExecutable(stagingDir, logChan)
	if swapErr != nil {
		if logChan != nil {
			logChan <- fmt.Sprintf("[Hinweis] Executable-Tausch: %v (fahre fort...)", swapErr)
		}
	}

	if engine != nil {
		if logChan != nil {
			logChan <- "[Update] Schritt 6/6: Prüfe und aktualisiere Container-Stack..."
		}
		if containers, _ := engine.GetContainers(ctx); len(containers) > 0 {
			_ = engine.Update(ctx, logChan)
		}
	}

	if logChan != nil {
		logChan <- "[Update] ✅ Aktualisierung erfolgreich abgeschlossen! Der Launcher startet jetzt neu..."
	}

	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmpDst := dst + ".tmp_update"
	out, err := os.OpenFile(tmpDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmpDst)
		return err
	}
	out.Close()

	if err := os.Rename(tmpDst, dst); err != nil {
		// On Windows, if destination exists, rename might fail with Access Denied; try remove then rename
		_ = os.Remove(dst)
		if renameErr := os.Rename(tmpDst, dst); renameErr != nil {
			_ = os.Remove(tmpDst)
			return renameErr
		}
	}
	return nil
}
