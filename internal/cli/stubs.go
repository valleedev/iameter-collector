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

func cmdPair(args []string) int {
	fs := flag.NewFlagSet("iameter pair", flag.ContinueOnError)
	_ = registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return notImplemented("pair", "Phase 5")
}

func cmdSync(args []string) int {
	fs := flag.NewFlagSet("iameter sync", flag.ContinueOnError)
	_ = registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return notImplemented("sync", "Phase 5/6")
}

func cmdDaemon(args []string) int {
	fs := flag.NewFlagSet("iameter daemon", flag.ContinueOnError)
	_ = registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return notImplemented("daemon", "Phase 6")
}

func cmdUnpair(args []string) int {
	fs := flag.NewFlagSet("iameter unpair", flag.ContinueOnError)
	_ = registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return notImplemented("unpair", "Phase 5")
}
