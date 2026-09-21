package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLI_Help(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--help"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected help output to mention OpenLocalCRM, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "status") || !strings.Contains(buf.String(), "install") {
		t.Errorf("expected help to list subcommands, got: %s", buf.String())
	}
}

func TestRunCLI_Version(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--version"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	if !strings.Contains(buf.String(), "OpenLocalCRM") {
		t.Errorf("expected version output, got: %s", buf.String())
	}
}

func TestRunCLI_UnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"foobar"}, t.TempDir(), &buf)
	if !handled || code != 2 {
		t.Fatalf("expected handled=true code=2 for unknown command, got %v, %d", handled, code)
	}
}

func TestRunCLI_GuiHandled(t *testing.T) {
	var buf bytes.Buffer
	handled, _ := ExecuteCommand([]string{"gui"}, t.TempDir(), &buf)
	if handled {
		t.Errorf("expected gui command to return handled=false to launch server, got true")
	}
}

func TestRunCLI_Versions(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"versions"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	output := buf.String()
	if !strings.Contains(output, "Verfügbare Versionen") || !strings.Contains(output, "v0.9") {
		t.Errorf("expected versions output containing v0.9, got:\n%s", output)
	}
}

func TestRunCLI_InstallHelpHasVersion(t *testing.T) {
	var buf bytes.Buffer
	handled, code := ExecuteCommand([]string{"--help"}, t.TempDir(), &buf)
	if !handled || code != 0 {
		t.Fatalf("expected handled=true code=0, got handled=%v code=%d", handled, code)
	}
	output := buf.String()
	if !strings.Contains(output, "--version, --tag TAG") {
		t.Errorf("expected help output to mention --version / --tag flag, got:\n%s", output)
	}
}

func TestRunCLI_SettingsExportAndImport(t *testing.T) {
	tmp := t.TempDir()

	// Create test .env
	initialEnv := `PORT=9090
INITIAL_ADMIN_EMAIL=cliadmin@solar.de
AI_PROVIDER=ollama
OLLAMA_MODEL=gemma4:12b
`
	_ = os.WriteFile(filepath.Join(tmp, ".env"), []byte(initialEnv), 0600)

	settingsJSON := filepath.Join(tmp, "exported.json")

	// 1. Export settings
	var exportBuf bytes.Buffer
	handled, code := ExecuteCommand([]string{"export-settings", settingsJSON}, tmp, &exportBuf)
	if !handled || code != 0 {
		t.Fatalf("expected export-settings handled=true code=0, got %v, %d: %s", handled, code, exportBuf.String())
	}
	if !strings.Contains(exportBuf.String(), "erfolgreich") {
		t.Errorf("expected success message, got: %s", exportBuf.String())
	}

	// 2. Modify exported file
	data, _ := os.ReadFile(settingsJSON)
	modified := strings.Replace(string(data), "cliadmin@solar.de", "modified@solar.de", 1)
	_ = os.WriteFile(settingsJSON, []byte(modified), 0600)

	// 3. Import settings back
	var importBuf bytes.Buffer
	handled, code = ExecuteCommand([]string{"import-settings", settingsJSON}, tmp, &importBuf)
	if !handled || code != 0 {
		t.Fatalf("expected import-settings handled=true code=0, got %v, %d: %s", handled, code, importBuf.String())
	}

	// 4. Verify updated .env
	newEnv, _ := os.ReadFile(filepath.Join(tmp, ".env"))
	if !strings.Contains(string(newEnv), "INITIAL_ADMIN_EMAIL=modified@solar.de") {
		t.Errorf("expected updated email in .env, got:\n%s", string(newEnv))
	}

	// 5. Test export-settings default path
	var defaultExportBuf bytes.Buffer
	handled, code = ExecuteCommand([]string{"export-settings"}, tmp, &defaultExportBuf)
	if !handled || code != 0 {
		t.Errorf("expected default export-settings to succeed, got %v, %d", handled, code)
	}

	// 6. Test import-settings errors
	var errImportBuf bytes.Buffer
	handled, code = ExecuteCommand([]string{"import-settings"}, tmp, &errImportBuf)
	if !handled || code == 0 {
		t.Errorf("expected error for missing import-settings arg")
	}

	var missingFileBuf bytes.Buffer
	handled, code = ExecuteCommand([]string{"import-settings", filepath.Join(tmp, "missing.json")}, tmp, &missingFileBuf)
	if !handled || code == 0 {
		t.Errorf("expected error for non-existent file import")
	}

	// 7. Test install with --settings-file and --settings-file=
	var installBuf bytes.Buffer
	installTmp := t.TempDir()
	handled, code = ExecuteCommand([]string{"install", "-y", "--dry-run", "--settings-file", settingsJSON, "--port=9099"}, installTmp, &installBuf)
	if !handled || code != 0 {
		t.Errorf("expected install with --settings-file to succeed in dry-run mode, got code %d: %s", code, installBuf.String())
	}

	var installBuf2 bytes.Buffer
	installTmp2 := t.TempDir()
	handled, code = ExecuteCommand([]string{"install", "-y", "--dry-run", "--settings-file=" + settingsJSON}, installTmp2, &installBuf2)
	if !handled || code != 0 {
		t.Errorf("expected install with --settings-file= to succeed in dry-run mode, got code %d: %s", code, installBuf2.String())
	}
}
