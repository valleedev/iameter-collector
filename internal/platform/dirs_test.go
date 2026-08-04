package platform

import (
	"path/filepath"
	"testing"
)

func TestDefaultDirsLinuxXDGOverride(t *testing.T) {
	if OS() != "linux" {
		t.Skip("XDG override test only meaningful on linux")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdgtest/config")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdgtest/data")
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdgtest/cache")

	d := DefaultDirs()
	if d.ConfigDir != filepath.Join("/tmp/xdgtest/config", "iameter") {
		t.Errorf("ConfigDir = %q", d.ConfigDir)
	}
	if d.DataDir != filepath.Join("/tmp/xdgtest/data", "iameter") {
		t.Errorf("DataDir = %q", d.DataDir)
	}
	if d.CacheDir != filepath.Join("/tmp/xdgtest/cache", "iameter") {
		t.Errorf("CacheDir = %q", d.CacheDir)
	}
}

func TestBinaryName(t *testing.T) {
	name := BinaryName()
	if IsWindows() && name != "iameter.exe" {
		t.Errorf("BinaryName() = %q on windows, want iameter.exe", name)
	}
	if !IsWindows() && name != "iameter" {
		t.Errorf("BinaryName() = %q, want iameter", name)
	}
}

func TestOSArchNonEmpty(t *testing.T) {
	if OS() == "" || Arch() == "" {
		t.Error("OS()/Arch() returned empty string")
	}
}
