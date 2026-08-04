package cli

import (
	"flag"

	"github.com/iameter/collector/internal/config"
)

// globalFlags holds the 6 global flags (section 10) registered on every
// subcommand's FlagSet so they work regardless of position, e.g. both
// `iameter --json status` and `iameter status --json`.
type globalFlags struct {
	apiBaseURL string
	configDir  string
	dataDir    string
	logLevel   string
	jsonOut    bool
	noColor    bool
}

func registerGlobalFlags(fs *flag.FlagSet) *globalFlags {
	g := &globalFlags{}
	fs.StringVar(&g.apiBaseURL, "api-base-url", "", "Backend base URL (env IAMETER_API_BASE_URL)")
	fs.StringVar(&g.configDir, "config-dir", "", "Override config directory")
	fs.StringVar(&g.dataDir, "data-dir", "", "Override data directory")
	fs.StringVar(&g.logLevel, "log-level", "", "debug|info|warn|error|silent (default info)")
	fs.BoolVar(&g.jsonOut, "json", false, "Emit machine-readable JSON output")
	fs.BoolVar(&g.noColor, "no-color", false, "Disable ANSI color output")
	return g
}

func (g *globalFlags) resolve() config.Options {
	return config.Resolve(g.apiBaseURL, g.configDir, g.dataDir, g.logLevel, g.jsonOut, g.noColor)
}
