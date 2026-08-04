// Package device generates and names the local device identifier.
//
// A device_id is generated locally on first run so statusline capture never
// has to block on network/pairing (section 6, section 11.10). If the device
// later pairs successfully, the backend's own device_id (returned by
// POST /v1/devices/pair) supersedes the local one for future snapshots —
// the backend is the source of truth once a device record exists there.
package device

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"strings"
)

// NewID generates a locally-unique device identifier of the form
// "dev_XXXXXXXXXXXX" (12 base32 chars, no padding, lowercase).
func NewID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("device: generate id: %w", err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return "dev_" + strings.ToLower(enc), nil
}

// DefaultName returns a best-effort local display name for this device,
// used only as the human label sent at pairing time (section 16). It is
// never included in usage sync payloads (section 17 forbids hostnames
// there).
func DefaultName() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "unknown-device"
	}
	return name
}
