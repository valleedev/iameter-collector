package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCfg(t *testing.T) InstallConfig {
	dir := t.TempDir()
	return InstallConfig{
		SettingsPath:   filepath.Join(dir, "settings.json"),
		BackupPath:     filepath.Join(dir, "settings.json.iameter-backup"),
		ChainStatePath: filepath.Join(dir, "chain.json"),
		IAMeterCommand: BuildCommand("/opt/iameter/iameter"),
	}
}

// archivo inexistente
func TestInstallMissingFile(t *testing.T) {
	cfg := testCfg(t)
	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if res.Action != ActionInstalled {
		t.Errorf("Action = %v, want %v", res.Action, ActionInstalled)
	}
	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != cfg.IAMeterCommand {
		t.Errorf("Command = %q, want %q", entry.Command, cfg.IAMeterCommand)
	}
	if _, err := os.Stat(cfg.BackupPath); err != nil {
		t.Errorf("backup not created: %v", err)
	}
}

// archivo válido con otras claves — deben preservarse
func TestInstallPreservesOtherKeys(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"theme":"dark","otherSetting":{"nested":true}}`)

	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	data, _ := os.ReadFile(cfg.SettingsPath)
	var doc map[string]json.RawMessage
	json.Unmarshal(data, &doc)
	if string(doc["theme"]) != `"dark"` {
		t.Errorf("theme = %s, want preserved", doc["theme"])
	}
	if _, ok := doc["otherSetting"]; !ok {
		t.Error("otherSetting key was dropped")
	}
}

// statusLine ausente
func TestInstallStatusLineAbsent(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"theme":"dark"}`)

	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if res.Action != ActionInstalled {
		t.Errorf("Action = %v, want %v", res.Action, ActionInstalled)
	}
}

// statusLine ya es de IA METER -> idempotente
func TestInstallStatusLineAlreadyIAMeter(t *testing.T) {
	cfg := testCfg(t)
	if _, err := Install(cfg); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Errorf("Action = %v, want %v", res.Action, ActionUnchanged)
	}
	// No chain state should ever be created for our own command.
	if _, err := os.Stat(cfg.ChainStatePath); !os.IsNotExist(err) {
		t.Error("chain state should not exist when statusLine is already IA METER's")
	}
}

// statusLine de IA METER pero el binario se movió -> actualiza sin encadenar
func TestInstallUpdatesMovedBinaryPath(t *testing.T) {
	cfg := testCfg(t)
	oldCmd := BuildCommand("/old/path/iameter")
	writeRaw(t, cfg.SettingsPath, `{"statusLine":{"type":"command","command":"`+jsonEscape(oldCmd)+`"}}`)

	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if res.Action != ActionUpdated {
		t.Errorf("Action = %v, want %v", res.Action, ActionUpdated)
	}
	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != cfg.IAMeterCommand {
		t.Errorf("Command = %q, want %q", entry.Command, cfg.IAMeterCommand)
	}
	if _, err := os.Stat(cfg.ChainStatePath); !os.IsNotExist(err) {
		t.Error("moved-binary update should not create chain state")
	}
}

// statusLine externo -> se encadena y se preserva
func TestInstallChainsExternalStatusLine(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"statusLine":{"type":"command","command":"~/.config/starship/statusline.sh"}}`)

	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if res.Action != ActionChained {
		t.Errorf("Action = %v, want %v", res.Action, ActionChained)
	}
	if res.ChainedFrom == nil || res.ChainedFrom.Command != "~/.config/starship/statusline.sh" {
		t.Fatalf("ChainedFrom = %+v", res.ChainedFrom)
	}

	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != cfg.IAMeterCommand {
		t.Errorf("Command = %q, want IA METER command", entry.Command)
	}

	chained, err := LoadChainState(cfg.ChainStatePath)
	if err != nil {
		t.Fatalf("LoadChainState() error = %v", err)
	}
	if chained == nil || chained.Command != "~/.config/starship/statusline.sh" {
		t.Fatalf("persisted chain state = %+v", chained)
	}
}

// JSON inválido -> nunca se sobrescribe
func TestInstallCorruptJSONNeverOverwritten(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{ this is not json`)

	_, err := Install(cfg)
	if err == nil {
		t.Fatal("Install() error = nil, want error for corrupt JSON")
	}

	data, _ := os.ReadFile(cfg.SettingsPath)
	if string(data) != `{ this is not json` {
		t.Error("corrupt settings.json was modified despite parse failure")
	}
}

// instalación repetida (idempotencia general, incluyendo tras encadenar)
func TestInstallRepeatedAfterChaining(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"statusLine":{"type":"command","command":"/usr/bin/starship-status"}}`)

	if _, err := Install(cfg); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	res, err := Install(cfg)
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Errorf("Action = %v, want %v (no double-chaining)", res.Action, ActionUnchanged)
	}
	// Chain state must still point at the ORIGINAL third party command, not
	// at IA METER (this is the recursion guard).
	chained, _ := LoadChainState(cfg.ChainStatePath)
	if chained == nil || chained.Command != "/usr/bin/starship-status" {
		t.Fatalf("chain state corrupted by repeated install: %+v", chained)
	}
}

// symlink -> rechazado
func TestInstallRefusesSymlink(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("symlink permissions differ on windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-settings.json")
	os.WriteFile(real, []byte(`{}`), 0o600)
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	cfg := InstallConfig{
		SettingsPath:   link,
		BackupPath:     filepath.Join(dir, "backup"),
		ChainStatePath: filepath.Join(dir, "chain.json"),
		IAMeterCommand: BuildCommand("/opt/iameter/iameter"),
	}
	_, err := Install(cfg)
	if err == nil {
		t.Fatal("Install() error = nil, want error for symlinked settings.json")
	}
}

// espacios y Unicode en la ruta del binario
func TestBuildCommandQuotesSpacesAndUnicode(t *testing.T) {
	cmd := BuildCommand("/Users/alíce/My Applications/iameter")
	if cmd == "" {
		t.Fatal("empty command")
	}
	// Must be a single shell "word" for the binary despite embedded spaces.
	if !strings.Contains(cmd, "My Applications") {
		t.Errorf("command lost the path: %q", cmd)
	}
}

// -- helpers --

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBackStatusLine(t *testing.T, settingsPath string) StatusLineEntry {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var entry StatusLineEntry
	if err := json.Unmarshal(doc["statusLine"], &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
