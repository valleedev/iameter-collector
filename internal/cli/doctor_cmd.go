package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iameter/collector/internal/config"
	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/daemon"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/settings"
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

	if claudeCodeDetected() {
		checks = append(checks, doctorCheck{Name: "Claude Code detection", Status: checkOK, Detail: "found"})
	} else {
		checks = append(checks, doctorCheck{Name: "Claude Code detection", Status: checkWarn, Detail: "~/.claude not found and `claude` not on PATH"})
	}

	checks = append(checks, statusLineCheck())

	checks = append(checks, queueCheck(opts.DataDir))
	checks = append(checks, credentialStoreCheck(opts.DataDir))
	checks = append(checks, backendConnectivityCheck(opts.APIBaseURL, opts.IsDefaultDevBackend()))
	checks = append(checks, daemonCheck())

	return checks
}

func queueCheck(dataDir string) doctorCheck {
	q, err := queue.Open(dataDir)
	if err != nil {
		return doctorCheck{Name: "Local queue", Status: checkErr, Detail: err.Error()}
	}
	n, err := q.Len()
	if err != nil {
		return doctorCheck{Name: "Local queue", Status: checkErr, Detail: err.Error()}
	}
	if n == 0 {
		return doctorCheck{Name: "Local queue", Status: checkWarn, Detail: "no snapshots captured yet — send a message in Claude Code"}
	}
	return doctorCheck{Name: "Local queue", Status: checkOK, Detail: fmt.Sprintf("%d pending snapshot(s)", n)}
}

func credentialStoreCheck(dataDir string) doctorCheck {
	store := credentials.New(dataDir)
	if store.IsFallback() {
		return doctorCheck{Name: "Credential store", Status: checkWarn,
			Detail: "no OS-native secret store available, using " + store.Name()}
	}
	return doctorCheck{Name: "Credential store", Status: checkOK, Detail: store.Name()}
}

func backendConnectivityCheck(apiBaseURL string, isDevDefault bool) doctorCheck {
	client := httpclient.New(apiBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		// An unreachable *default* dev backend just means no `iameter
		// mock-server` is running — expected out of the box, not an error.
		status := checkErr
		if isDevDefault {
			status = checkWarn
		}
		return doctorCheck{Name: "Backend connectivity", Status: status, Detail: "unreachable: " + err.Error()}
	}
	return doctorCheck{Name: "Backend connectivity", Status: checkOK, Detail: apiBaseURL}
}

func daemonCheck() doctorCheck {
	st, err := daemon.NewServiceManager().Status()
	if err != nil {
		return doctorCheck{Name: "Daemon", Status: checkErr, Detail: err.Error()}
	}
	switch {
	case st.Running:
		return doctorCheck{Name: "Daemon", Status: checkOK, Detail: st.Detail}
	case st.Installed:
		return doctorCheck{Name: "Daemon", Status: checkWarn, Detail: "registered but not running: " + st.Detail}
	default:
		return doctorCheck{Name: "Daemon", Status: checkWarn, Detail: "not registered — run `iameter install`. " + st.Detail}
	}
}

func statusLineCheck() doctorCheck {
	settingsPath, err := settings.Path()
	if err != nil {
		return doctorCheck{Name: "StatusLine configuration", Status: checkErr, Detail: err.Error()}
	}
	entry, hasEntry, err := settings.CurrentStatusLine(settingsPath)
	switch {
	case errors.Is(err, settings.ErrCorruptJSON):
		return doctorCheck{Name: "StatusLine configuration", Status: checkErr, Detail: "settings.json is not valid JSON"}
	case errors.Is(err, settings.ErrSymlink):
		return doctorCheck{Name: "StatusLine configuration", Status: checkErr, Detail: "settings.json is a symlink, refusing to read through it"}
	case err != nil:
		return doctorCheck{Name: "StatusLine configuration", Status: checkErr, Detail: err.Error()}
	case !hasEntry:
		return doctorCheck{Name: "StatusLine configuration", Status: checkWarn, Detail: "not configured — run `iameter install`"}
	case settings.IsIAMeterCommand(entry.Command):
		return doctorCheck{Name: "StatusLine configuration", Status: checkOK, Detail: "configured"}
	default:
		return doctorCheck{Name: "StatusLine configuration", Status: checkWarn, Detail: "points at another command — run `iameter install` to chain IA METER onto it"}
	}
}
