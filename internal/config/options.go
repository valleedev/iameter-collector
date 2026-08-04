// Package config resolves global CLI options (flags/env/defaults) and
// loads/saves the small persisted device config file. It does not store
// secrets — device tokens live in internal/credentials.
package config

import (
	"os"

	"github.com/valleedev/iameter-collector/internal/logging"
	"github.com/valleedev/iameter-collector/internal/platform"
)

// DefaultAPIBaseURL points at the local mock dev server (internal/mockserver)
// started via `iameter mock-server` or the standalone mock binary. It is
// intentionally not a real hosted domain — section 21/30 forbid presenting
// an unowned or placeholder domain as production. Real deployments must set
// IAMETER_API_BASE_URL or pass --api-base-url explicitly.
const DefaultAPIBaseURL = "http://127.0.0.1:8787"

// Options are the global flags accepted by every subcommand (section 10).
type Options struct {
	APIBaseURL string
	ConfigDir  string
	DataDir    string
	LogLevel   logging.Level
	JSON       bool
	NoColor    bool
}

// Resolve applies precedence flag > env > default for each option.
// Flag values are passed in already parsed; empty string/zero-value means
// "not set on the command line".
func Resolve(flagAPIBaseURL, flagConfigDir, flagDataDir, flagLogLevel string, flagJSON, flagNoColor bool) Options {
	dirs := platform.DefaultDirs()

	apiBaseURL := flagAPIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = os.Getenv("IAMETER_API_BASE_URL")
	}
	if apiBaseURL == "" {
		apiBaseURL = DefaultAPIBaseURL
	}

	configDir := flagConfigDir
	if configDir == "" {
		configDir = dirs.ConfigDir
	}

	dataDir := flagDataDir
	if dataDir == "" {
		dataDir = dirs.DataDir
	}

	level := logging.LevelInfo
	if flagLogLevel != "" {
		level = logging.ParseLevel(flagLogLevel)
	}

	noColor := flagNoColor
	if !noColor {
		if os.Getenv("NO_COLOR") != "" {
			noColor = true
		}
	}

	return Options{
		APIBaseURL: apiBaseURL,
		ConfigDir:  configDir,
		DataDir:    dataDir,
		LogLevel:   level,
		JSON:       flagJSON,
		NoColor:    noColor,
	}
}

// IsDefaultDevBackend reports whether the resolved API base URL is still
// the built-in local mock default, i.e. the user has not configured a real
// backend.
func (o Options) IsDefaultDevBackend() bool {
	return o.APIBaseURL == DefaultAPIBaseURL
}
