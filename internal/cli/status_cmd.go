package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iameter/collector/internal/config"
	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/version"
)

func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("iameter status", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	dc, err := config.LoadDeviceConfig(opts.ConfigDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter: load device config:", err)
		return 1
	}

	st := buildStatusReport(opts, dc)
	fillQueueStatus(&st, opts.DataDir)

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(st))
	}

	printStatusHuman(st)
	return 0
}

// statusReport is filled in incrementally as later phases add daemon and
// syncer introspection (Phase 5/6). Queue fields (Phase 4) are real.
type statusReport struct {
	Paired           bool     `json:"paired"`
	DeviceName       string   `json:"device_name,omitempty"`
	Provider         string   `json:"provider"`
	CollectorVersion string   `json:"collector_version"`
	OS               string   `json:"os"`
	Arch             string   `json:"arch"`
	DaemonRunning    bool     `json:"daemon_running"`
	QueuePending     int      `json:"queue_pending"`
	LastSnapshotAt   string   `json:"last_snapshot_at,omitempty"`
	LastSyncAt       string   `json:"last_sync_at,omitempty"`
	FiveHourPct      *float64 `json:"five_hour_used_percentage,omitempty"`
	FiveHourResetsAt int64    `json:"five_hour_resets_at,omitempty"`
	SevenDayPct      *float64 `json:"seven_day_used_percentage,omitempty"`
	SevenDayResetsAt int64    `json:"seven_day_resets_at,omitempty"`
	APIBaseURL       string   `json:"api_base_url"`
	DevBackend       bool     `json:"dev_backend"`
}

func buildStatusReport(opts config.Options, dc config.DeviceConfig) statusReport {
	name := dc.DeviceName
	if name == "" {
		name = "(unnamed)"
	}
	return statusReport{
		Paired:           dc.Paired,
		DeviceName:       name,
		Provider:         "claude",
		CollectorVersion: version.Version,
		OS:               platform.OS(),
		Arch:             platform.Arch(),
		APIBaseURL:       opts.APIBaseURL,
		DevBackend:       opts.IsDefaultDevBackend(),
	}
}

// fillQueueStatus adds the pending count and most recent snapshot to st.
// Errors opening/reading the queue are non-fatal for `status` — an
// unreadable queue shouldn't stop the rest of the report from printing.
func fillQueueStatus(st *statusReport, dataDir string) {
	q, err := queue.Open(dataDir)
	if err != nil {
		return
	}
	pending, err := q.Pending()
	if err != nil {
		return
	}
	st.QueuePending = len(pending)
	if len(pending) == 0 {
		return
	}
	last := pending[len(pending)-1]
	st.LastSnapshotAt = last.Snapshot.CapturedAt
	if fh := last.Snapshot.RateLimits.FiveHour; fh != nil {
		pct := fh.UsedPercentage
		st.FiveHourPct = &pct
		st.FiveHourResetsAt = fh.ResetsAt
	}
	if sd := last.Snapshot.RateLimits.SevenDay; sd != nil {
		pct := sd.UsedPercentage
		st.SevenDayPct = &pct
		st.SevenDayResetsAt = sd.ResetsAt
	}
}

func formatResetTime(unixSeconds int64) string {
	if unixSeconds <= 0 {
		return "unknown"
	}
	return time.Unix(unixSeconds, 0).UTC().Format(time.RFC3339)
}

func printStatusHuman(st statusReport) {
	fmt.Println("IA METER Collector", st.CollectorVersion)
	fmt.Println()
	if st.Paired {
		fmt.Printf("Pairing:       paired (%s)\n", st.DeviceName)
	} else {
		fmt.Println("Pairing:       not paired — run `iameter pair <CODE>`")
	}
	fmt.Println("Provider:      " + st.Provider)
	fmt.Printf("Platform:      %s/%s\n", st.OS, st.Arch)
	if st.DaemonRunning {
		fmt.Println("Daemon:        running")
	} else {
		fmt.Println("Daemon:        not running")
	}
	fmt.Printf("Pending sync:  %d item(s)\n", st.QueuePending)

	switch {
	case st.FiveHourPct != nil:
		fmt.Printf("5h usage:      %.1f%% (resets %s)\n", *st.FiveHourPct, formatResetTime(st.FiveHourResetsAt))
	default:
		fmt.Println("5h usage:      no data yet")
	}
	switch {
	case st.SevenDayPct != nil:
		fmt.Printf("7d usage:      %.1f%% (resets %s)\n", *st.SevenDayPct, formatResetTime(st.SevenDayResetsAt))
	default:
		fmt.Println("7d usage:      no data yet")
	}

	if st.LastSnapshotAt != "" {
		fmt.Println("Last snapshot: " + st.LastSnapshotAt)
	}

	if st.DevBackend {
		fmt.Println()
		fmt.Println("[WARN] Using the local development backend (" + st.APIBaseURL + ").")
		fmt.Println("       Set IAMETER_API_BASE_URL or --api-base-url for a real backend.")
	}
}
