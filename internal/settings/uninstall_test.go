package settings

import (
	"os"
	"testing"
)

func uninstallCfg(cfg InstallConfig) UninstallConfig {
	return UninstallConfig{SettingsPath: cfg.SettingsPath, ChainStatePath: cfg.ChainStatePath}
}

// desinstalación tras instalación limpia (sin statusLine previo) -> elimina la clave
func TestUninstallRemovesWhenNoPriorStatusLine(t *testing.T) {
	cfg := testCfg(t)
	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	res, err := Uninstall(uninstallCfg(cfg))
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.Action != UninstallRemoved {
		t.Errorf("Action = %v, want %v", res.Action, UninstallRemoved)
	}
	if _, hasExisting, _ := readStatusLine(mustLoad(t, cfg.SettingsPath)); hasExisting {
		t.Error("statusLine key still present after uninstall")
	}
}

// restauración de un statusLine externo previamente encadenado
func TestUninstallRestoresChainedStatusLine(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"statusLine":{"type":"command","command":"/usr/bin/starship-status"}}`)
	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	res, err := Uninstall(uninstallCfg(cfg))
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.Action != UninstallRestoredChain {
		t.Errorf("Action = %v, want %v", res.Action, UninstallRestoredChain)
	}
	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != "/usr/bin/starship-status" {
		t.Errorf("Command = %q, want restored third-party command", entry.Command)
	}
	if _, err := os.Stat(cfg.ChainStatePath); !os.IsNotExist(err) {
		t.Error("chain state file should be removed after restoring")
	}
}

// desinstalación cuando el usuario cambió el statusLine manualmente -> no se toca
func TestUninstallLeavesUnrelatedStatusLineAlone(t *testing.T) {
	cfg := testCfg(t)
	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	// user manually repoints statusLine after install
	writeRaw(t, cfg.SettingsPath, `{"statusLine":{"type":"command","command":"/usr/bin/something-else"}}`)

	res, err := Uninstall(uninstallCfg(cfg))
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.Action != UninstallNotOurs {
		t.Errorf("Action = %v, want %v", res.Action, UninstallNotOurs)
	}
	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != "/usr/bin/something-else" {
		t.Errorf("Command = %q, user's manual change was overwritten", entry.Command)
	}
}

// desinstalación idempotente: ejecutar dos veces no falla
func TestUninstallIdempotent(t *testing.T) {
	cfg := testCfg(t)
	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := Uninstall(uninstallCfg(cfg)); err != nil {
		t.Fatalf("first Uninstall() error = %v", err)
	}
	res, err := Uninstall(uninstallCfg(cfg))
	if err != nil {
		t.Fatalf("second Uninstall() error = %v", err)
	}
	if res.Action != UninstallNoop {
		t.Errorf("Action = %v, want %v", res.Action, UninstallNoop)
	}
}

// desinstalación sin haber instalado nunca -> no error
func TestUninstallNeverInstalled(t *testing.T) {
	cfg := testCfg(t)
	res, err := Uninstall(uninstallCfg(cfg))
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if res.Action != UninstallNoop {
		t.Errorf("Action = %v, want %v", res.Action, UninstallNoop)
	}
}

// full round trip preserves unrelated keys
func TestInstallUninstallRoundTripPreservesOtherKeys(t *testing.T) {
	cfg := testCfg(t)
	writeRaw(t, cfg.SettingsPath, `{"theme":"dark","statusLine":{"type":"command","command":"/usr/bin/starship-status"}}`)

	if _, err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := Uninstall(uninstallCfg(cfg)); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	doc := mustLoad(t, cfg.SettingsPath)
	if string(doc["theme"]) != `"dark"` {
		t.Errorf("theme = %s, want preserved through install+uninstall", doc["theme"])
	}
	entry := readBackStatusLine(t, cfg.SettingsPath)
	if entry.Command != "/usr/bin/starship-status" {
		t.Errorf("Command = %q, want restored", entry.Command)
	}
}

func mustLoad(t *testing.T, path string) raw {
	t.Helper()
	doc, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
