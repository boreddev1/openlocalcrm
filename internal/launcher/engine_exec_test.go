package launcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// All tests in this file drive the Engine through the execFn seam: the docker
// argv is asserted on the recorder, never executed against the host daemon.

func TestNewEngineWiresPlatformExecDefault(t *testing.T) {
	engine := NewEngine(t.TempDir())
	if engine.execFn == nil {
		t.Fatal("NewEngine must wire execFn to the platform execCommand default so production keeps executing real commands")
	}
	cmd := engine.command(context.Background(), "docker", "info")
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if got := strings.Join(cmd.Args, " "); got != "docker info" {
		t.Errorf("unexpected argv %q, want %q", got, "docker info")
	}
	// The command is deliberately never started: no daemon is touched here.
}

func TestEngineCommandFallsBackToPlatformDefault(t *testing.T) {
	e := &Engine{BaseDir: t.TempDir()}
	cmd := e.command(context.Background(), "docker", "version")
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if got := strings.Join(cmd.Args, " "); got != "docker version" {
		t.Errorf("unexpected argv %q, want %q", got, "docker version")
	}
	// Again: constructed only, never started.
}

func TestEngineUpRecordsComposeUpAndStreamsLogs(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	rec, stub := newExecStub(t, map[string]string{
		"docker compose up -d --remove-orphans": "Container crm-db Started\nContainer crm-server Started\n",
	})
	engine.execFn = stub

	logChan := make(chan string, 16)
	if err := engine.Up(context.Background(), logChan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(logChan)

	var lines []string
	for line := range logChan {
		lines = append(lines, line)
	}
	wantLines := []string{"Container crm-db Started", "Container crm-server Started"}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Errorf("expected compose output to be streamed to logChan, got %v", lines)
	}

	wantCmds := []string{"docker compose up -d --remove-orphans"}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("Up issued %v, want %v", got, wantCmds)
	}
	up := rec.mustFind(t, "docker", "compose", "up", "-d", "--remove-orphans")
	if up.cmd.Dir != tmpDir {
		t.Errorf("expected compose up to run in BaseDir %q, got %q", tmpDir, up.cmd.Dir)
	}
}

func TestEngineComposeLifecycleCommands(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Engine) error
		want []string
	}{
		{"stop", func(e *Engine) error { return e.Stop(context.Background()) }, []string{"docker", "compose", "stop"}},
		{"restart", func(e *Engine) error { return e.Restart(context.Background()) }, []string{"docker", "compose", "restart"}},
		{"down", func(e *Engine) error { return e.Down(context.Background()) }, []string{"docker", "compose", "down"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			engine := NewEngine(tmpDir)
			// The lifecycle command must *succeed* (its error is returned),
			// so it gets a stand-in process; empty output is fine.
			rec, stub := newExecStub(t, map[string]string{strings.Join(c.want, " "): ""})
			engine.execFn = stub

			if err := c.run(engine); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := rec.recorded()
			if len(got) != 1 || got[0] != strings.Join(c.want, " ") {
				t.Errorf("recorded %v, want exactly [%v]", got, strings.Join(c.want, " "))
			}
			cmd := rec.mustFind(t, c.want...)
			if cmd.cmd.Dir != tmpDir {
				t.Errorf("expected command to run in BaseDir %q, got %q", tmpDir, cmd.cmd.Dir)
			}
		})
	}
}

func TestEngineGetContainersFromComposePS(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	rec, stub := newExecStub(t, map[string]string{
		"docker compose ps --format json": `[{"name":"crm-db","service":"db","state":"running","status":"Up 2 hours","health":"healthy"}]`,
	})
	engine.execFn = stub

	list, err := engine.GetContainers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "crm-db" || list[0].Service != "db" || list[0].Health != "healthy" {
		t.Fatalf("unexpected container list: %+v", list)
	}

	// The compose-ps fast path must short-circuit the fallback sweeps.
	wantCmds := []string{"docker compose ps --format json"}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}
}

func TestEngineGetContainersFallsBackToKnownContainers(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	// compose ps succeeds with empty output (so JSON parsing fails and the
	// fallback chain runs); the fallback queries fail instantly without a
	// process, like docker would when no daemon is installed.
	rec, stub := newExecStub(t, map[string]string{"docker compose ps --format json": ""})
	engine.execFn = stub

	_, err := engine.GetContainers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no containers found") {
		t.Fatalf("expected 'no containers found', got: %v", err)
	}

	wantCmds := []string{
		"docker compose ps --format json",
		"docker ps -a --format {{json .}}",
		"docker inspect crm-proxy",
		"docker inspect crm-server",
		"docker inspect crm-worker",
		"docker inspect crm-db",
	}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}
}

func TestEngineRecoverEnvFromDocker(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	inspectJSON := `[{"Name":"/crm-db","State":{"Status":"running","Health":{"Status":"healthy"}},` +
		`"Config":{"Labels":{"com.docker.compose.service":"db"},"Env":["POSTGRES_PASSWORD=stub_pw","POSTGRES_USER=stub_user","POSTGRES_DB=stub_db"]}}]`
	rec, stub := newExecStub(t, map[string]string{
		"docker ps -a --format {{.Names}}": "crm-db\nnginx\n",
		"docker inspect crm-db":            inspectJSON,
	})
	engine.execFn = stub

	env, err := engine.RecoverEnvFromDocker(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env["DB_PASSWORD"] != "stub_pw" || env["DB_USER"] != "stub_user" || env["DB_NAME"] != "stub_db" {
		t.Errorf("unexpected recovered env: %v", env)
	}

	// The sweep may only add name-matched containers; "nginx" is dropped and
	// "crm-db" is already a static candidate (inspected exactly once).
	wantCmds := []string{
		"docker ps -a --format {{.Names}}",
		"docker inspect crm-db",
		"docker inspect crm-server",
		"docker inspect crm-proxy",
		"docker inspect crm-worker",
		"docker inspect openlocalcrm-db-1",
		"docker inspect openlocalcrm-server-1",
		"docker inspect openlocalcrm-caddy-1",
	}
	got := rec.recorded()
	if !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}
	for _, cmd := range got {
		if cmd == "docker inspect nginx" {
			t.Errorf("unrelated container must not be inspected, got %q", cmd)
		}
	}
}

func TestEngineCreateBackup(t *testing.T) {
	tmpDir := t.TempDir()
	persistentDir := filepath.Join(tmpDir, "persistent")
	t.Setenv("OPENLOCALCRM_BACKUP_DIR", persistentDir)

	engine := NewEngine(tmpDir)
	rec, stub := newExecStub(t, map[string]string{
		"docker compose exec -T db pg_dump -U postgres --clean --if-exists openlocalcrm": "-- stub dump\n",
	})
	engine.execFn = stub

	filename, err := engine.CreateBackup(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(filename, "openlocalcrm_backup_") || !strings.HasSuffix(filename, ".sql") {
		t.Fatalf("unexpected backup filename %q", filename)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "backups", filename))
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	// The file starts with the launcher's backup header, followed by whatever
	// the (stubbed) pg_dump process wrote to stdout.
	content := string(data)
	if !strings.Contains(content, "-- OpenLocalCRM Database Backup") || !strings.Contains(content, "-- stub dump\n") {
		t.Errorf("unexpected backup content %q", data)
	}
	if _, err := os.Stat(filepath.Join(persistentDir, filename)); err != nil {
		t.Errorf("expected persistent mirror of backup: %v", err)
	}

	wantCmds := []string{"docker compose exec -T db pg_dump -U postgres --clean --if-exists openlocalcrm"}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}
	dump := rec.mustFind(t, strings.Fields(wantCmds[0])...)
	if dump.cmd.Dir != tmpDir {
		t.Errorf("expected pg_dump to run in BaseDir %q, got %q", tmpDir, dump.cmd.Dir)
	}
}

func TestEngineRestoreBackupSequence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("OPENLOCALCRM_BACKUP_DIR", filepath.Join(tmpDir, "persistent"))

	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatalf("failed to prepare backups dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupsDir, "restore_me.sql"), []byte("-- SCHEMA_VERSION: 00042_restore_test.sql\nSELECT 1;\n"), 0644); err != nil {
		t.Fatalf("failed to write fixture backup: %v", err)
	}

	engine := NewEngine(tmpDir)
	// The psql import streams the dump through Start/Wait and its failure
	// would abort the restore, so it must succeed; every other step may fail
	// like it would against an unreachable daemon without changing the flow.
	rec, stub := newExecStub(t, map[string]string{
		"docker compose exec -T db psql -U postgres -d openlocalcrm": "",
	})
	engine.execFn = stub

	if err := engine.RestoreBackup(context.Background(), "restore_me.sql", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cleanSQL := "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO postgres; GRANT ALL ON SCHEMA public TO public; CREATE EXTENSION IF NOT EXISTS vector;"
	wantCmds := []string{
		"docker compose exec -T db pg_dump -U postgres --clean --if-exists openlocalcrm",
		"docker compose stop server worker",
		"docker compose exec -T db psql -U postgres -d openlocalcrm -c " + cleanSQL,
		"docker compose exec -T db psql -U postgres -d openlocalcrm",
		"docker compose up -d server worker",
	}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}

	// The SQL dump must be piped into the import psql process.
	importCmd := rec.mustFind(t, "docker", "compose", "exec", "-T", "db", "psql", "-U", "postgres", "-d", "openlocalcrm")
	if importCmd.cmd.Stdin == nil {
		t.Error("expected restore import to receive the dump on stdin")
	}
}

func TestEngineUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("OPENLOCALCRM_BACKUP_DIR", filepath.Join(tmpDir, "persistent"))

	engine := NewEngine(tmpDir)
	rec, stub := newExecStub(t, map[string]string{
		"docker compose up -d --build --remove-orphans": "Built crm-server\n",
	})
	engine.execFn = stub

	if err := engine.Update(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update must snapshot first, then rebuild via compose.
	wantCmds := []string{
		"docker compose exec -T db pg_dump -U postgres --clean --if-exists openlocalcrm",
		"docker compose up -d --build --remove-orphans",
	}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}
}

func TestEngineResetAdminPasswordBootstrap(t *testing.T) {
	tmpDir := t.TempDir()
	engine := NewEngine(tmpDir)
	rec, stub := newExecStub(t, map[string]string{
		"docker compose exec -T db pg_isready -U postgres -d openlocalcrm": "",
		"docker compose exec -T db psql -U postgres -d openlocalcrm":       "UPDATE 1\n",
	})
	engine.execFn = stub

	if err := engine.ResetAdminPassword(context.Background(), "Admin@Test.LOCAL", "Passw0rd123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCmds := []string{
		"docker compose up -d db",
		"docker compose exec -T db pg_isready -U postgres -d openlocalcrm",
		"docker compose exec -T db psql -U postgres -d openlocalcrm",
	}
	if got := rec.recorded(); !reflect.DeepEqual(got, wantCmds) {
		t.Errorf("recorded %v, want %v", got, wantCmds)
	}

	// The admin SQL (with the argon2 hash) must be delivered via stdin.
	psql := rec.mustFind(t, "docker", "compose", "exec", "-T", "db", "psql", "-U", "postgres", "-d", "openlocalcrm")
	if psql.cmd.Stdin == nil {
		t.Error("expected reset-password SQL to be piped to psql on stdin")
	}
}
