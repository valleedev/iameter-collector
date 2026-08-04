package cli

import (
	"flag"
	"fmt"
	"os"
)

// notImplemented prints a clear, honest message (never silently pretends to
// succeed) and returns exit code 1. Each stub here is replaced with a real
// implementation in the phase noted.
func notImplemented(cmdName, phase string) int {
	fmt.Fprintf(os.Stderr, "iameter %s: not yet implemented (lands in %s)\n", cmdName, phase)
	return 1
}

func cmdDaemon(args []string) int {
	fs := flag.NewFlagSet("iameter daemon", flag.ContinueOnError)
	_ = registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return notImplemented("daemon", "Phase 6")
}
