package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iameter/collector/internal/fsutil"
)

// DeviceConfig is the small non-secret state IA METER persists about this
// installation. The device token itself is never stored here — it lives in
// internal/credentials.
type DeviceConfig struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Paired     bool   `json:"paired"`
	UserID     string `json:"user_id,omitempty"`
	PairedAt   string `json:"paired_at,omitempty"`
}

const deviceConfigFile = "device.json"

// LoadDeviceConfig reads the device config from configDir. A missing file
// is not an error: it returns a zero-value DeviceConfig so first-run flows
// can proceed.
func LoadDeviceConfig(configDir string) (DeviceConfig, error) {
	path := filepath.Join(configDir, deviceConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DeviceConfig{}, nil
		}
		return DeviceConfig{}, fmt.Errorf("config: read device config: %w", err)
	}
	var dc DeviceConfig
	if err := json.Unmarshal(data, &dc); err != nil {
		return DeviceConfig{}, fmt.Errorf("config: parse device config: %w", err)
	}
	return dc, nil
}

// SaveDeviceConfig writes the device config atomically (write temp file,
// fsync, rename) with restrictive permissions.
func SaveDeviceConfig(configDir string, dc DeviceConfig) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("config: create config dir: %w", err)
	}
	data, err := json.MarshalIndent(dc, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal device config: %w", err)
	}
	path := filepath.Join(configDir, deviceConfigFile)
	return fsutil.AtomicWriteFile(path, data, 0o600)
}
