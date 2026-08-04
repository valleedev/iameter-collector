package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDeviceConfigMissing(t *testing.T) {
	dc, err := LoadDeviceConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadDeviceConfig() error = %v, want nil for missing file", err)
	}
	if dc.Paired {
		t.Errorf("LoadDeviceConfig() on missing file returned Paired=true")
	}
	if dc.DeviceID != "" {
		t.Errorf("LoadDeviceConfig() on missing file returned non-empty DeviceID")
	}
}

func TestSaveLoadDeviceConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := DeviceConfig{
		DeviceID:   "dev_abc123",
		DeviceName: "test-device",
		Paired:     true,
		UserID:     "usr_xyz",
		PairedAt:   "2026-08-03T00:00:00Z",
	}
	if err := SaveDeviceConfig(dir, want); err != nil {
		t.Fatalf("SaveDeviceConfig() error = %v", err)
	}
	got, err := LoadDeviceConfig(dir)
	if err != nil {
		t.Fatalf("LoadDeviceConfig() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestSaveDeviceConfigCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "config")
	if err := SaveDeviceConfig(dir, DeviceConfig{DeviceID: "dev_x"}); err != nil {
		t.Fatalf("SaveDeviceConfig() error = %v", err)
	}
	got, err := LoadDeviceConfig(dir)
	if err != nil {
		t.Fatalf("LoadDeviceConfig() error = %v", err)
	}
	if got.DeviceID != "dev_x" {
		t.Errorf("DeviceID = %q, want dev_x", got.DeviceID)
	}
}

func TestLoadDeviceConfigCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, deviceConfigFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeviceConfig(dir); err == nil {
		t.Error("LoadDeviceConfig() on corrupt JSON: want error, got nil")
	}
}
