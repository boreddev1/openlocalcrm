package launcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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
	t.Setenv("OPENLOCALCRM_BACKUP_DIR", filepath.Join(tmpDir, "persistent_backups"))
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
					"AI_EMBEDDING_MODEL=text-embedding-3-small",
					"OLLAMA_EMBEDDING_MODEL=qwen3-embedding:0.6b",
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
	if env["AI_EMBEDDING_MODEL"] != "text-embedding-3-small" {
		t.Errorf("expected AI_EMBEDDING_MODEL pass-through, got %s", env["AI_EMBEDDING_MODEL"])
	}
	if env["OLLAMA_EMBEDDING_MODEL"] != "qwen3-embedding:0.6b" {
		t.Errorf("expected OLLAMA_EMBEDDING_MODEL pass-through, got %s", env["OLLAMA_EMBEDDING_MODEL"])
	}
}

func TestConvertInspectToContainerInfo(t *testing.T) {
	items := []DockerInspectItem{
		{
			Name: "/crm-server",
			State: struct {
				Status string `json:"Status"`
				Health struct {
					Status string `json:"Status"`
				} `json:"Health"`
			}{Status: "running", Health: struct {
				Status string `json:"Status"`
			}{Status: "healthy"}},
		},
		{
			Name: "/crm-db",
			State: struct {
				Status string `json:"Status"`
				Health struct {
					Status string `json:"Status"`
				} `json:"Health"`
			}{Status: "exited"},
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

	// The engine no longer talks to the host Docker daemon in tests: every
	// command is recorded and answered by a test-binary stand-in (see
	// newExecStub / TestLauncherExecHelperProcess). The canned "daemon
	// inventory" below mixes stack-owned names with unrelated containers and
	// volumes so the test can prove the dynamic sweeps are name-scoped.
	rec, stub := newExecStub(t, map[string]string{
		"docker ps -a --format {{.Names}}":    "crm-db\n/crm-slashy\nopenlocalcrm-worker-2\nmavalio-proxy-1\nCRM-WeirdCase\nnginx\npostgres16\n",
		"docker volume ls --format {{.Name}}": "crm_pg_data\nopenlocalcrm_crm_keys\nmavalio_backup\nnginx_cache\n",
	})
	engine.execFn = stub

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

	// Verify logs were emitted, including the documented reset milestones.
	if len(logs) == 0 {
		t.Errorf("expected log messages during ResetAll")
	}
	for _, milestone := range []string{
		"[Reset] Starte vollständigen Factory Reset...",
		"[Reset] Bereinige verbleibende CRM-Container...",
		"[Reset] Bereinige persistente Docker-Volumes...",
		"[Reset] Lokale Konfiguration (.env) entfernt.",
		"[Reset] ✅ Factory Reset erfolgreich abgeschlossen.",
	} {
		found := false
		for _, line := range logs {
			if strings.Contains(line, milestone) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ResetAll log to contain %q, got:\n%s", milestone, strings.Join(logs, "\n"))
		}
	}

	// The factory reset must keep issuing the genuine, thorough command
	// sequence (this is the product feature — nothing was softened), but the
	// test observes it through the recorded argv instead of executing it on a
	// live daemon.
	staticContainers := []string{
		"crm-db", "crm-server", "crm-proxy", "crm-worker",
		"openlocalcrm-db-1", "openlocalcrm-server-1", "openlocalcrm-caddy-1", "openlocalcrm-worker-1",
		"mavalio-db-1", "mavalio-server-1", "mavalio-caddy-1", "mavalio-worker-1",
	}
	staticVolumes := []string{
		"openlocalcrm_pg_data", "crm_pg_data", "mavalio_pg_data",
		"openlocalcrm_crm_storage", "crm_storage", "mavalio_crm_storage",
		"openlocalcrm_crm_keys", "crm_keys", "mavalio_crm_keys",
		"openlocalcrm_crm_caddy_data", "crm_caddy_data", "mavalio_crm_caddy_data",
		"openlocalcrm_crm_caddy_config", "crm_caddy_config", "mavalio_crm_caddy_config",
	}
	// Names matched by the dynamic sweeps — "/crm-slashy" proves the leading
	// slash trim, "CRM-WeirdCase" proves the case-insensitive match — while
	// nginx/postgres16/nginx_cache must never be touched.
	dynamicContainers := []string{"crm-db", "crm-slashy", "openlocalcrm-worker-2", "mavalio-proxy-1", "CRM-WeirdCase"}
	dynamicVolumes := []string{"crm_pg_data", "openlocalcrm_crm_keys", "mavalio_backup"}

	want := []string{"docker compose down -v --remove-orphans"}
	for _, name := range staticContainers {
		want = append(want, "docker rm -f "+name)
	}
	want = append(want, "docker ps -a --format {{.Names}}")
	for _, name := range dynamicContainers {
		want = append(want, "docker rm -f "+name)
	}
	for _, vol := range staticVolumes {
		want = append(want, "docker volume rm -f "+vol)
	}
	want = append(want, "docker volume ls --format {{.Name}}")
	for _, vol := range dynamicVolumes {
		want = append(want, "docker volume rm -f "+vol)
	}

	got := rec.recorded()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ResetAll issued the wrong docker command sequence.\nwant:\n  %s\ngot:\n  %s",
			strings.Join(want, "\n  "), strings.Join(got, "\n  "))
	}

	for _, noise := range []string{"docker rm -f nginx", "docker rm -f postgres16", "docker volume rm -f nginx_cache"} {
		rec.mustNotRecord(t, strings.Fields(noise)...)
	}

	// compose down is scoped to the engine's BaseDir, not some global context.
	down := rec.mustFind(t, "docker", "compose", "down", "-v", "--remove-orphans")
	if down.cmd.Dir != tmpDir {
		t.Errorf("expected compose down to run in BaseDir %q, got %q", tmpDir, down.cmd.Dir)
	}

}
