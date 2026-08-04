package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/version"
)

func cmdVersion(args []string) int {
	fs := flag.NewFlagSet("iameter version", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(map[string]string{
			"version":    version.Version,
			"commit":     version.Commit,
			"build_date": version.BuildDate,
			"os":         platform.OS(),
			"arch":       platform.Arch(),
		}))
	}

	fmt.Println(version.Info())
	return 0
}

func zeroOr(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter:", err)
		return 1
	}
	return 0
}
