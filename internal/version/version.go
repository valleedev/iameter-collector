// Package version holds build-time metadata injected via -ldflags.
package version

import "fmt"

// These are overridden at build time via:
//
//	go build -ldflags "-X github.com/iameter/collector/internal/version.Version=... \
//	  -X github.com/iameter/collector/internal/version.Commit=... \
//	  -X github.com/iameter/collector/internal/version.BuildDate=..."
var (
	Version   = "0.1.0-dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is the full version string shown by `iameter version`.
func Info() string {
	return fmt.Sprintf("iameter %s (commit %s, built %s, %s)", Version, Commit, BuildDate, RuntimeInfo())
}
