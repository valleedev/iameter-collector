package settings

import (
	"strings"

	"github.com/valleedev/iameter-collector/internal/platform"
)

// BuildCommand returns the exact command string to install as the
// statusLine "command" field, given the absolute path to the iameter
// binary. It must be quoted per-platform because Claude Code invokes it
// through a shell — an unquoted path containing spaces (very common on
// Windows and macOS, e.g. "Application Support") would otherwise split
// into multiple words (section 31: compatibility with spaced/Unicode
// paths).
func BuildCommand(binaryPath string) string {
	return quotePath(binaryPath) + " statusline"
}

// quotePath quotes an absolute path for safe inclusion in a shell command
// string. This only ever wraps a known-good binary path IA METER resolved
// itself via os.Executable — never externally supplied input — so this is
// quoting for correctness (spaces/Unicode), not an attempt to neutralize
// untrusted data.
func quotePath(path string) string {
	if platform.IsWindows() {
		// cmd.exe / PowerShell: wrap in double quotes, escape embedded
		// double quotes by doubling them.
		return `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
	}
	// POSIX sh: wrap in single quotes, escape embedded single quotes using
	// the standard '\'' trick (close quote, escaped quote, reopen quote).
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
