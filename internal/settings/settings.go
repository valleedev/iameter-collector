package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/valleedev/iameter-collector/internal/fsutil"
)

// ErrCorruptJSON is returned when settings.json exists but does not parse.
// Callers must never overwrite the file when this is returned (section 13:
// "Si el JSON está corrupto, no lo sobrescribas").
var ErrCorruptJSON = errors.New("settings: existing settings.json is not valid JSON")

// ErrSymlink is returned when settings.json is a symlink. IA METER refuses
// to follow it rather than risk writing through an attacker-controlled
// link (section 24: symlink attacks).
var ErrSymlink = errors.New("settings: settings.json is a symlink, refusing to modify it")

// StatusLineEntry mirrors the statusLine object in Claude Code's
// settings.json: {"type": "command", "command": "...", "padding": N}.
type StatusLineEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Padding *int   `json:"padding,omitempty"`
}

// raw is the generic settings.json document: every key IA METER doesn't
// understand is kept as untouched raw JSON so installing/uninstalling never
// drops unrelated user configuration (section 13: "No reemplaces todo el
// archivo de configuración").
type raw map[string]json.RawMessage

// load reads and parses settingsPath. A missing file returns an empty
// document, not an error — there's nothing to preserve or corrupt. A
// symlinked or malformed file returns a sentinel error the caller must
// treat as fatal (never proceed to overwrite).
func load(settingsPath string) (raw, error) {
	info, err := os.Lstat(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return raw{}, nil
		}
		return nil, fmt.Errorf("settings: stat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlink
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("settings: read: %w", err)
	}
	if len(data) == 0 {
		return raw{}, nil
	}

	var doc raw
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, ErrCorruptJSON
	}
	if doc == nil {
		doc = raw{}
	}
	return doc, nil
}

// save writes doc to settingsPath atomically. Key ordering is not
// preserved (Go's json.Marshal on a map sorts keys alphabetically) — an
// acceptable trade-off for an MVP; a byte-for-byte order-preserving JSON
// writer would be meaningfully more code for no functional benefit, since
// settings.json has no ordering semantics.
func save(settingsPath string, doc raw) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	data = append(data, '\n')
	return fsutil.AtomicWriteFile(settingsPath, data, 0o600)
}

// backupIfAbsent copies the current settings.json to backupPath the first
// time IA METER touches it, so uninstall can restore the true pre-install
// state even across multiple install runs. It never overwrites an existing
// backup (that would erase the real "before IA METER" snapshot).
func backupIfAbsent(settingsPath, backupPath string) error {
	if _, err := os.Stat(backupPath); err == nil {
		return nil // already have a pristine snapshot
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("settings: stat backup: %w", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing existed before IA METER; an empty backup file marks
			// that fact so uninstall knows to remove settings.json's
			// statusLine key rather than "restore" from nothing.
			return fsutil.AtomicWriteFile(backupPath, []byte{}, 0o600)
		}
		return fmt.Errorf("settings: read for backup: %w", err)
	}
	return fsutil.AtomicWriteFile(backupPath, data, 0o600)
}
