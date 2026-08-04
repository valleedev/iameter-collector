// Package platform provides canonical OS/arch identification and the
// per-OS filesystem locations IA METER uses, so every other package
// (config, credentials, settings, daemon, installer) shares one source
// of truth instead of re-deriving paths.
package platform

import "runtime"

// OS returns the canonical OS identifier: "linux", "darwin", or "windows".
func OS() string {
	return runtime.GOOS
}

// Arch returns the canonical architecture identifier: "amd64" or "arm64".
func Arch() string {
	return runtime.GOARCH
}

func IsWindows() bool { return runtime.GOOS == "windows" }
func IsDarwin() bool  { return runtime.GOOS == "darwin" }
func IsLinux() bool   { return runtime.GOOS == "linux" }
