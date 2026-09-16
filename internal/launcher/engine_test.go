package launcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseContainersJSON(t *testing.T) {
	rawJSON := `[
		{"Name":"mavalio-server-1","Service":"crm-server","State":"running","Status":"Up 2 hours (healthy)","Health":"healthy"},
		{"Name":"mavalio-db-1","Service":"crm-db","State":"running","Status":"Up 2 hours (healthy)","Health":"healthy"}
	]`

	containers, err := ParseContainersJSON([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].Service != "crm-server" || containers[0].Health != "healthy" {
		t.Errorf("unexpected container data: %+v", containers[0])
	}
	if containers[1].Service != "crm-db" || containers[1].Health != "healthy" {
		t.Errorf("unexpected container data: %+v", containers[1])
	}
}

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	backupsDir := filepath.Join(tmpDir, "backups")
	_ = os.MkdirAll(backupsDir, 0755)
	_ = os.WriteFile(filepath.Join(backupsDir, "test_backup.sql"), []byte("SELECT 1;"), 0644)

	list, err := engine.ListBackups()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(list))
	}
	if list[0].Filename != "test_backup.sql" {
		t.Errorf("expected filename test_backup.sql, got %s", list[0].Filename)
	}
}

func TestRestoreBackup_SecurityTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	err := engine.RestoreBackup(context.Background(), "../secret.sql", nil)
	if err == nil {
		t.Errorf("expected error on directory traversal filename")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		b        int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
	}
	for _, c := range cases {
		got := formatBytes(c.b)
		if got != c.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", c.b, got, c.expected)
		}
	}
}

func TestInspectBackupFile(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	backupsDir := filepath.Join(tmpDir, "backups")
	_ = os.MkdirAll(backupsDir, 0755)

	sqlContent := `-- OpenLocalCRM Database Backup
-- SCHEMA_VERSION: 00008_notes_schema.sql
INSERT INTO schema_migrations (version) VALUES ('00001_initial_schema.sql');
INSERT INTO schema_migrations (version) VALUES ('00008_notes_schema.sql');
`
	_ = os.WriteFile(filepath.Join(backupsDir, "versioned.sql"), []byte(sqlContent), 0644)

	meta := engine.InspectBackupFile("versioned.sql")
	if meta.SchemaVersion != "00008_notes_schema.sql" {
		t.Errorf("expected schema version 00008_notes_schema.sql, got %s", meta.SchemaVersion)
	}
}

func TestParseDockerInspectJSON_AndExtractEnv(t *testing.T) {
	rawInspect := `[
		{
			"Name": "/crm-db",
			"State": { "Status": "running", "Health": { "Status": "healthy" } },
			"Config": {
				"Labels": { "com.docker.compose.service": "db" },
				"Env": [
					"POSTGRES_USER=postgres",
					"POSTGRES_PASSWORD=super_secret_db_pass",
					"POSTGRES_DB=mavalio"
				]
			}
		},
		{
			"Name": "/crm-server",
			"State": { "Status": "running", "Health": { "Status": "healthy" } },
			"Config": {
				"Labels": { "com.docker.compose.service": "server" },
				"Env": [
					"DATABASE_URL=postgres://postgres:super_secret_db_pass@db:5432/mavalio?sslmode=disable",
					"INITIAL_ADMIN_EMAIL=custom_admin@example.com",
					"INITIAL_ADMIN_PASSWORD=my_admin_pass",
					"CONNECTOR_API_TOKEN=conn_secret_token_123",
					"AI_PROVIDER=openai",
					"AI_API_KEY=sk-test-key-abc",
					"DEMO_MODE=false"
				]
			}
		},
		{
			"Name": "/crm-proxy",
			"State": { "Status": "running" },
			"Config": { "Labels": { "com.docker.compose.service": "caddy" } },
			"NetworkSettings": {
				"Ports": {
					"80/tcp": [{ "HostPort": "8080" }]
				}
			}
		}
	]`

	items, err := ParseDockerInspectJSON([]byte(rawInspect))
	if err != nil {
		t.Fatalf("failed parsing inspect json: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	env := ExtractEnvFromInspect(items)
	if env["DB_PASSWORD"] != "super_secret_db_pass" {
		t.Errorf("expected DB_PASSWORD super_secret_db_pass, got %s", env["DB_PASSWORD"])
	}
	if env["INITIAL_ADMIN_EMAIL"] != "custom_admin@example.com" {
		t.Errorf("expected INITIAL_ADMIN_EMAIL custom_admin@example.com, got %s", env["INITIAL_ADMIN_EMAIL"])
	}
	if env["CONNECTOR_API_TOKEN"] != "conn_secret_token_123" {
		t.Errorf("expected CONNECTOR_API_TOKEN conn_secret_token_123, got %s", env["CONNECTOR_API_TOKEN"])
	}
	if env["APP_PORT"] != "8080" {
		t.Errorf("expected APP_PORT 8080, got %s", env["APP_PORT"])
	}
}

func TestConvertInspectToContainerInfo(t *testing.T) {
	items := []DockerInspectItem{
		{
			Name:  "/crm-server",
			State: struct { Status string `json:"Status"`; Health struct { Status string `json:"Status"` } `json:"Health"` }{ Status: "running", Health: struct { Status string `json:"Status"` }{ Status: "healthy" } },
		},
		{
			Name:  "/crm-db",
			State: struct { Status string `json:"Status"`; Health struct { Status string `json:"Status"` } `json:"Health"` }{ Status: "exited" },
		},
	}

	containers := ConvertInspectToContainerInfo(items)
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].Name != "crm-server" || containers[0].Service != "server" || containers[0].State != "running" {
		t.Errorf("unexpected server container info: %+v", containers[0])
	}
	if containers[1].Name != "crm-db" || containers[1].Service != "db" || containers[1].State != "exited" {
		t.Errorf("unexpected db container info: %+v", containers[1])
	}
}

func TestGetPersistentBackupDir(t *testing.T) {
	dir := GetPersistentBackupDir()
	if dir == "" {
		t.Errorf("expected non-empty persistent backup directory")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected persistent backup directory to be created: %s", dir)
	}
}

func TestResetAdminPassword_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	// 1. Empty email error
	err := engine.ResetAdminPassword(context.Background(), "", "ValidPass123!")
	if err == nil || !strings.Contains(err.Error(), "E-Mail") {
		t.Errorf("expected error for empty email, got: %v", err)
	}

	// 2. Short password error
	err = engine.ResetAdminPassword(context.Background(), "admin@test.local", "123")
	if err == nil || !strings.Contains(err.Error(), "mindestens 6") {
		t.Errorf("expected error for short password, got: %v", err)
	}
}

func TestResetAll(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)

	// Create a dummy .env file
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("INITIAL_ADMIN_EMAIL=test@test.local\n"), 0644); err != nil {
		t.Fatalf("failed to create dummy .env: %v", err)
	}

	logChan := make(chan string, 50)
	var logs []string
	done := make(chan struct{})
	go func() {
		for line := range logChan {
			logs = append(logs, line)
		}
		close(done)
	}()

	err := engine.ResetAll(context.Background(), logChan)
	close(logChan)
	<-done

	if err != nil {
		t.Errorf("expected ResetAll to succeed, got: %v", err)
	}

	// Verify .env file was removed
	if _, statErr := os.Stat(envFile); !os.IsNotExist(statErr) {
		t.Errorf("expected .env to be deleted, but it still exists")
	}

	// Verify logs were emitted
	if len(logs) == 0 {
		t.Errorf("expected log messages during ResetAll")
	}
}



