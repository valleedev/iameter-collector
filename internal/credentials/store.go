// Package credentials stores secrets (the device token from pairing)
// using the best available OS-native secret store, falling back to a
// restrictive-permission file when none is available (section 18).
//
// Tokens are never logged, never written to Claude Code's settings, never
// passed as command-line arguments, and never exposed to child processes
// (see internal/statusline.RunChained's env filtering).
package credentials

import "errors"

// ErrNotFound is returned by Load when key has no stored value.
var ErrNotFound = errors.New("credentials: not found")

// Store is the credential storage contract (section 18).
type Store interface {
	Save(key string, value []byte) error
	Load(key string) ([]byte, error)
	Delete(key string) error

	// Name identifies the backing store for diagnostics, e.g.
	// "linux-secret-service", "macos-keychain", "windows-dpapi",
	// "file-fallback".
	Name() string

	// IsFallback reports whether this is the restrictive-file fallback,
	// so callers (doctor, install) can warn the user (section 18).
	IsFallback() bool
}

// New selects the best available store for the current OS, falling back
// to a file under fallbackDir (typically the data directory) if no
// OS-native store is usable.
func New(fallbackDir string) Store {
	if store, ok := platformStore(fallbackDir); ok {
		return store
	}
	return newFileStore(fallbackDir)
}
