package launcher

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerStatusEndpoint(t *testing.T) {
	engine := NewEngine(t.TempDir())
	handler := NewServer(t.TempDir(), engine)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestServerPreflightEndpoint(t *testing.T) {
	engine := NewEngine(t.TempDir())
	handler := NewServer(t.TempDir(), engine)

	req := httptest.NewRequest("POST", "/api/preflight", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestServerLogsEndpoint(t *testing.T) {
	engine := NewEngine(t.TempDir())
	handler := NewServer(t.TempDir(), engine)

	req := httptest.NewRequest("GET", "/api/logs", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestServerBackupsListEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("GET", "/api/backups", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestServerInstallEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("POST", "/api/install/wsl", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Errorf("expected route /api/install/wsl to be handled")
	}
}

func TestServerBackupRestoreEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("POST", "/api/backup/restore", strings.NewReader(`{"filename":"test.sql"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestServerControlUpdateEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("POST", "/api/control/update", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestServerBackupDownloadEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	backupsDir := filepath.Join(tmpDir, "backups")
	_ = os.MkdirAll(backupsDir, 0755)
	_ = os.WriteFile(filepath.Join(backupsDir, "sample_dump.sql"), []byte("-- dump data"), 0644)

	// 1. Successful download
	req := httptest.NewRequest("GET", "/api/backup/download?file=sample_dump.sql", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "sample_dump.sql") {
		t.Errorf("expected Content-Disposition header with sample_dump.sql, got %s", w.Header().Get("Content-Disposition"))
	}
	if w.Body.String() != "-- dump data" {
		t.Errorf("unexpected body content: %s", w.Body.String())
	}

	// 2. Directory traversal blocked
	reqTraversal := httptest.NewRequest("GET", "/api/backup/download?file=../evil.sql", nil)
	wTraversal := httptest.NewRecorder()
	handler.ServeHTTP(wTraversal, reqTraversal)

	if wTraversal.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for directory traversal, got %d", wTraversal.Code)
	}

	// 3. Not found
	reqNotFound := httptest.NewRequest("GET", "/api/backup/download?file=nonexistent.sql", nil)
	wNotFound := httptest.NewRecorder()
	handler.ServeHTTP(wNotFound, reqNotFound)

	if wNotFound.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for nonexistent file, got %d", wNotFound.Code)
	}
}

func TestServerResetAdminPasswordEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	// 1. Missing password
	reqEmpty := httptest.NewRequest("POST", "/api/admin/reset-password", strings.NewReader(`{"email":"admin@openlocalcrm.local","password":""}`))
	wEmpty := httptest.NewRecorder()
	handler.ServeHTTP(wEmpty, reqEmpty)

	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty password, got %d", wEmpty.Code)
	}

	// 2. Invalid JSON
	reqBadJSON := httptest.NewRequest("POST", "/api/admin/reset-password", strings.NewReader(`not-json`))
	wBadJSON := httptest.NewRecorder()
	handler.ServeHTTP(wBadJSON, reqBadJSON)

	if wBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for bad json, got %d", wBadJSON.Code)
	}

	// 3. Method not allowed
	reqGet := httptest.NewRequest("GET", "/api/admin/reset-password", nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for GET, got %d", wGet.Code)
	}
}

func TestServerControlResetEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	// Create a dummy .env
	envFile := filepath.Join(tmpDir, ".env")
	_ = os.WriteFile(envFile, []byte("DEMO=true\n"), 0644)

	req := httptest.NewRequest("POST", "/api/control/reset", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Errorf("expected success:true in response, got %s", w.Body.String())
	}

	// Verify .env was deleted
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Errorf("expected .env to be deleted after reset")
	}
}

func TestServer_UpdateCheckRoute(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("GET", "/api/update/check", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "current_commit") {
		t.Errorf("expected response to contain current_commit, got %s", w.Body.String())
	}
}

func TestServer_UpdateExecuteRoute(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("POST", "/api/update/execute", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Errorf("expected success:true in response, got %s", w.Body.String())
	}

	time.Sleep(150 * time.Millisecond)
}

func TestServerVersionsEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	handler := NewServer(tmpDir, engine)

	req := httptest.NewRequest("GET", "/api/versions", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "versions") || !strings.Contains(body, "v0.9") {
		t.Errorf("expected versions response containing v0.9, got: %s", body)
	}
}


