// Package settings installs/uninstalls IA METER as (or chained onto) Claude
// Code's statusLine, per https://code.claude.com/docs/en/settings:
// user-level settings live at ~/.claude/settings.json on macOS/Linux and
// %USERPROFILE%\.claude\settings.json on Windows.
package settings

import (
	"os"
	"path/filepath"

	"github.com/iameter/collector/internal/platform"
)

// Path returns the absolute path to Claude Code's user-level settings.json.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if platform.IsWindows() {
		if up := os.Getenv("USERPROFILE"); up != "" {
			home = up
		}
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// BackupPath returns where the pre-IAMETER snapshot of settings.json is
// stored, alongside the original file.
func BackupPath(settingsPath string) string {
	return settingsPath + ".iameter-backup"
}

// DefaultChainStatePath returns where a preserved third-party statusLine
// command is stored, inside IA METER's own config directory (not next to
// Claude Code's settings.json, to keep IA METER's state self-contained).
func DefaultChainStatePath(configDir string) string {
	return filepath.Join(configDir, "chained_statusline.json")
}
