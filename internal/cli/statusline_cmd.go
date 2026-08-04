package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iameter/collector/internal/capture"
	"github.com/iameter/collector/internal/config"
	"github.com/iameter/collector/internal/device"
	"github.com/iameter/collector/internal/logging"
	"github.com/iameter/collector/internal/model"
	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/providers/claude"
	"github.com/iameter/collector/internal/statusline"
	"github.com/iameter/collector/internal/version"
)

// cmdStatusline implements section 11: read Claude Code's statusLine JSON
// from stdin, extract only the whitelisted rate_limits fields, and print a
// short status line. It never depends on network access and always prints
// *something* to stdout — a parse failure degrades to "Consumo no
// disponible" rather than leaving Claude Code's status bar blank or making
// the command exit non-zero (a broken statusLine command would visibly
// break Claude Code's UI, which is worse than showing stale/absent data).
//
// Queueing the snapshot for sync (section 11 point 9) is wired in once
// internal/queue exists (Phase 4); for now the snapshot is only built and
// rendered.
func cmdStatusline(args []string) int {
	fs := flag.NewFlagSet("iameter statusline", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()
	logger := logging.Default(opts.LogLevel)

	data, err := capture.ReadLimited(os.Stdin)
	if err != nil {
		logger.Warn("statusline: %v", err)
		fmt.Println(statusline.Render(model.RateLimits{}))
		return 0
	}

	rl, err := claude.New().Parse(bytes.NewReader(data))
	if err != nil {
		// Never log the raw payload — only that parsing failed.
		logger.Warn("statusline: parse failed: %v", err)
		fmt.Println(statusline.Render(model.RateLimits{}))
		return 0
	}

	if _, err := ensureDeviceID(opts.ConfigDir); err != nil {
		logger.Warn("statusline: could not persist device id: %v", err)
	}

	fmt.Println(statusline.Render(*rl))
	return 0
}

// buildSnapshot assembles the whitelisted outgoing payload (section 6/11)
// from a parsed RateLimits. Exported for reuse by Phase 4's queue wiring
// and Phase 5's sync tests.
func buildSnapshot(deviceID string, rl model.RateLimits) model.UsageSnapshot {
	return model.UsageSnapshot{
		DeviceID:         deviceID,
		Provider:         claude.Name,
		CollectorVersion: version.Version,
		CapturedAt:       time.Now().UTC().Format(time.RFC3339),
		Platform: model.Platform{
			OS:   platform.OS(),
			Arch: platform.Arch(),
		},
		RateLimits: rl,
	}
}

// ensureDeviceID loads the device config, generating and persisting a
// device_id on first run if one doesn't exist yet — statusline must work
// before pairing (section 6, section 11.10).
func ensureDeviceID(configDir string) (string, error) {
	dc, err := config.LoadDeviceConfig(configDir)
	if err != nil {
		return "", err
	}
	if dc.DeviceID != "" {
		return dc.DeviceID, nil
	}
	id, err := device.NewID()
	if err != nil {
		return "", err
	}
	dc.DeviceID = id
	if dc.DeviceName == "" {
		dc.DeviceName = device.DefaultName()
	}
	if err := config.SaveDeviceConfig(configDir, dc); err != nil {
		return "", err
	}
	return id, nil
}
