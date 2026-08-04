package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/iameter/collector/internal/config"
	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/version"
)

type checkStatus string

const (
	checkOK   checkStatus = "OK"
	checkWarn checkStatus = "WARN"
	checkErr  checkStatus = "ERROR"
)

type doctorCheck struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("iameter doctor", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	checks := runDoctorChecks(opts)

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(checks))
	}

	exitCode := 0
	for _, c := range checks {
		fmt.Printf("[%s] %s", c.Status, c.Name)
		if c.Detail != "" {
			fmt.Printf(" — %s", c.Detail)
		}
		fmt.Println()
		if c.Status == checkErr {
			exitCode = 1
		}
	}
	return exitCode
}

// runDoctorChecks performs the checks feasible with Phase 1's building
// blocks (platform, config). Claude Code detection/config, statusLine,
// daemon, backend connectivity, pairing, credential store, and queue checks
// are added as those subsystems land in later phases.
func runDoctorChecks(opts config.Options) []doctorCheck {
	var checks []doctorCheck

	checks = append(checks, doctorCheck{
		Name:   "Operating system",
		Status: checkOK,
		Detail: fmt.Sprintf("%s/%s", platform.OS(), platform.Arch()),
	})

	checks = append(checks, doctorCheck{
		Name:   "Collector version",
		Status: checkOK,
		Detail: version.Version,
	})

	if err := os.MkdirAll(opts.ConfigDir, 0o700); err != nil {
		checks = append(checks, doctorCheck{Name: "Config directory", Status: checkErr, Detail: err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "Config directory", Status: checkOK, Detail: opts.ConfigDir})
	}

	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		checks = append(checks, doctorCheck{Name: "Data directory", Status: checkErr, Detail: err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "Data directory", Status: checkOK, Detail: opts.DataDir})
	}

	dc, err := config.LoadDeviceConfig(opts.ConfigDir)
	if err != nil {
		checks = append(checks, doctorCheck{Name: "Device pairing", Status: checkErr, Detail: err.Error()})
	} else if dc.Paired {
		checks = append(checks, doctorCheck{Name: "Device pairing", Status: checkOK, Detail: "paired"})
	} else {
		checks = append(checks, doctorCheck{Name: "Device pairing", Status: checkWarn, Detail: "not paired — run `iameter pair <CODE>`"})
	}

	if opts.IsDefaultDevBackend() {
		checks = append(checks, doctorCheck{Name: "Backend URL", Status: checkWarn, Detail: "using local dev default " + opts.APIBaseURL})
	} else {
		checks = append(checks, doctorCheck{Name: "Backend URL", Status: checkOK, Detail: opts.APIBaseURL})
	}

	checks = append(checks, doctorCheck{Name: "Claude Code detection", Status: checkWarn, Detail: "not yet implemented (Phase 3)"})
	checks = append(checks, doctorCheck{Name: "StatusLine configuration", Status: checkWarn, Detail: "not yet implemented (Phase 3)"})
	checks = append(checks, doctorCheck{Name: "Local queue", Status: checkWarn, Detail: "not yet implemented (Phase 4)"})
	checks = append(checks, doctorCheck{Name: "Credential store", Status: checkWarn, Detail: "not yet implemented (Phase 5)"})
	checks = append(checks, doctorCheck{Name: "Backend connectivity", Status: checkWarn, Detail: "not yet implemented (Phase 5)"})
	checks = append(checks, doctorCheck{Name: "Daemon", Status: checkWarn, Detail: "not yet implemented (Phase 6)"})

	return checks
}
