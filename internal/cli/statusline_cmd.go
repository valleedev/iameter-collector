package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/valleedev/iameter-collector/internal/capture"
	"github.com/valleedev/iameter-collector/internal/config"
	"github.com/valleedev/iameter-collector/internal/device"
	"github.com/valleedev/iameter-collector/internal/logging"
	"github.com/valleedev/iameter-collector/internal/model"
	"github.com/valleedev/iameter-collector/internal/platform"
	"github.com/valleedev/iameter-collector/internal/providers/claude"
	"github.com/valleedev/iameter-collector/internal/queue"
	"github.com/valleedev/iameter-collector/internal/settings"
	"github.com/valleedev/iameter-collector/internal/statusline"
	"github.com/valleedev/iameter-collector/internal/version"
)

// cmdStatusline implements section 11: read Claude Code's statusLine JSON
// from stdin, extract only the whitelisted rate_limits fields, print a
// short status line, and queue the snapshot for sync. It never depends on
// network access and always prints *something* to stdout — a parse
// failure degrades to "Consumo no disponible" rather than leaving Claude
// Code's status bar blank or making the command exit non-zero (a broken
// statusLine command would visibly break Claude Code's UI, which is worse
// than showing stale/absent data). Likewise, a queue failure is logged and
// swallowed — it must never delay or break the printed status line.
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

	chainPath := settings.DefaultChainStatePath(opts.ConfigDir)
	chained, err := settings.LoadChainState(chainPath)
	if err != nil {
		logger.Warn("statusline: could not read chained statusLine state: %v", err)
	}

	// Parse our own usage data and (if a third-party statusLine was
	// preserved at install time) run it concurrently, feeding it the same
	// stdin JSON — section 13: capture usage in parallel while preserving
	// the previous tool's visual output.
	var wg sync.WaitGroup
	var rl *model.RateLimits
	var parseErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		rl, parseErr = claude.New().Parse(bytes.NewReader(data))
	}()

	var chainedOutput string
	var chainedErr error
	if chained != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chainedOutput, chainedErr = statusline.RunChained(context.Background(), chained.Command, data)
		}()
	}
	wg.Wait()

	if parseErr != nil {
		// Never log the raw payload — only that parsing failed.
		logger.Warn("statusline: parse failed: %v", parseErr)
		rl = &model.RateLimits{}
	}

	deviceID, err := ensureDeviceID(opts.ConfigDir)
	if err != nil {
		logger.Warn("statusline: could not persist device id: %v", err)
	}

	if parseErr == nil && deviceID != "" && !rl.Empty() {
		snapshot := buildSnapshot(deviceID, *rl)
		if err := config.SaveLastSnapshot(opts.ConfigDir, snapshot); err != nil {
			logger.Warn("statusline: could not cache last snapshot: %v", err)
		}
		enqueueSnapshot(opts.DataDir, snapshot, logger)
	}

	if chained != nil {
		if chainedErr == nil {
			fmt.Println(chainedOutput)
			return 0
		}
		logger.Warn("statusline: chained command failed, falling back to IA METER output: %v", chainedErr)
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

// enqueueSnapshot persists the parsed usage data to the local queue
// (section 11 point 9, section 14) so it survives offline periods until
// the daemon/syncer (Phase 5/6) can send it. Failures are logged, never
// fatal — statusline must finish fast regardless of disk issues.
func enqueueSnapshot(dataDir string, snapshot model.UsageSnapshot, logger *logging.Logger) {
	q, err := queue.Open(dataDir)
	if err != nil {
		logger.Warn("statusline: could not open queue: %v", err)
		return
	}
	if _, err := q.Enqueue(snapshot); err != nil {
		logger.Warn("statusline: could not enqueue snapshot: %v", err)
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
