// Package cli implements the iameter command router and subcommands.
package cli

import (
	"fmt"
	"os"
	"strings"
)

const usage = `IA METER Collector — usage data sync for Claude Code

Usage:
  iameter <command> [flags]

Commands:
  version      Show version, commit and build date
  status       Show pairing, sync and consumption status
  doctor       Diagnose the local installation
  statusline   Read Claude Code statusLine JSON from stdin, print status text
  pair         Pair this device with IA METER using a pairing code
  sync         Trigger one immediate sync attempt and exit
  daemon       Run the background sync daemon
  install      Install IA METER (binary, Claude Code statusLine, service)
  uninstall    Remove IA METER and restore previous configuration
  unpair       Remove local pairing credentials

Global flags (valid before or after the command):
  --api-base-url string   Backend base URL (env IAMETER_API_BASE_URL)
  --config-dir string     Override config directory
  --data-dir string       Override data directory
  --log-level string      debug|info|warn|error|silent (default info)
  --json                  Emit machine-readable JSON output
  --no-color              Disable ANSI color output
`

// globalValueFlags/globalBoolFlags list the section-10 global flags so Run
// can recognize them ahead of the subcommand token, e.g.
// `iameter --json status` as well as `iameter status --json`. Go's flag
// package only parses flags contiguous from the start of a FlagSet's args,
// so global flags given before the command are hoisted after it here.
var globalValueFlags = map[string]bool{
	"--api-base-url": true,
	"--config-dir":   true,
	"--data-dir":     true,
	"--log-level":    true,
}

var globalBoolFlags = map[string]bool{
	"--json":     true,
	"--no-color": true,
}

// splitArgs finds the first non-flag token as the command, hoisting any
// global flags that appeared before it to the front of the returned args
// so the command's FlagSet (which also registers the global flags) sees
// them.
func splitArgs(args []string) (cmd string, cmdArgs []string) {
	var hoisted []string
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		key := a
		if idx := strings.IndexByte(a, '='); idx >= 0 {
			key = a[:idx]
		}
		switch {
		case globalValueFlags[key] && !strings.Contains(a, "="):
			hoisted = append(hoisted, a)
			if i+1 < len(args) {
				i++
				hoisted = append(hoisted, args[i])
			}
		case globalValueFlags[key] || globalBoolFlags[key]:
			hoisted = append(hoisted, a)
		case strings.HasPrefix(a, "-"):
			// Unrecognized flag before the command; let the eventual
			// subcommand FlagSet report it as an error.
			hoisted = append(hoisted, a)
		default:
			cmd = a
			i++
			return cmd, append(hoisted, args[i:]...)
		}
	}
	return "", hoisted
}

// Run dispatches to the requested subcommand and returns a process exit code.
func Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := splitArgs(args)
	if cmd == "" {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch cmd {
	case "version":
		return cmdVersion(rest)
	case "status":
		return cmdStatus(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "statusline":
		return cmdStatusline(rest)
	case "pair":
		return cmdPair(rest)
	case "sync":
		return cmdSync(rest)
	case "daemon":
		return cmdDaemon(rest)
	case "install":
		return cmdInstall(rest)
	case "uninstall":
		return cmdUninstall(rest)
	case "unpair":
		return cmdUnpair(rest)
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "iameter: unknown command %q\n\n", cmd)
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
}
