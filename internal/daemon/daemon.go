// Package daemon runs IA METER's background sync loop (section 15):
// periodic sync attempts with exponential backoff and jitter, periodic
// heartbeats, a single-instance lock, and graceful shutdown on
// context cancellation (SIGINT/SIGTERM, wired by the CLI).
package daemon

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/valleedev/iameter-collector/internal/config"
	"github.com/valleedev/iameter-collector/internal/fsutil"
	"github.com/valleedev/iameter-collector/internal/logging"
	"github.com/valleedev/iameter-collector/internal/syncer"
)

// Defaults for production use. Tests override these via Config to run in
// milliseconds instead of minutes.
const (
	DefaultSyncInterval          = 30 * time.Second
	DefaultMaxBackoff            = 10 * time.Minute
	DefaultPausedRecheckInterval = 5 * time.Minute
	DefaultHeartbeatInterval     = 5 * time.Minute
	DefaultLockTimeout           = 2 * time.Second
)

type Config struct {
	Syncer    *syncer.Syncer
	ConfigDir string
	DataDir   string
	Logger    *logging.Logger

	SyncInterval          time.Duration
	MaxBackoff            time.Duration
	PausedRecheckInterval time.Duration
	HeartbeatInterval     time.Duration
	LockTimeout           time.Duration

	// OnTick, if set, is called after every sync attempt with the interval
	// chosen for the next one. Used only by tests to observe loop state
	// without sleeping through real timers.
	OnTick func(nextInterval time.Duration, paused bool)
}

func (c *Config) applyDefaults() {
	if c.SyncInterval == 0 {
		c.SyncInterval = DefaultSyncInterval
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = DefaultMaxBackoff
	}
	if c.PausedRecheckInterval == 0 {
		c.PausedRecheckInterval = DefaultPausedRecheckInterval
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if c.LockTimeout == 0 {
		c.LockTimeout = DefaultLockTimeout
	}
	if c.Logger == nil {
		c.Logger = logging.Default(logging.LevelInfo)
	}
}

// ErrAlreadyRunning means another daemon instance holds the lock (section
// 15: "evitar instancias duplicadas").
var ErrAlreadyRunning = errors.New("daemon: another instance is already running")

const lockFileName = "daemon.lock"

// Run blocks until ctx is canceled, running the sync and heartbeat loops.
// It returns nil on a clean shutdown, or ErrAlreadyRunning if another
// instance already holds the single-instance lock.
func Run(ctx context.Context, cfg Config) error {
	cfg.applyDefaults()

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("daemon: create data dir: %w", err)
	}

	lock, err := fsutil.AcquireFileLock(lockPath(cfg.DataDir), cfg.LockTimeout)
	if err != nil {
		return ErrAlreadyRunning
	}
	defer lock.Release()

	cfg.Logger.Info("daemon: started (pid lock acquired)")

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	syncTimer := time.NewTimer(0) // fire immediately on start
	defer syncTimer.Stop()
	heartbeatTimer := time.NewTimer(cfg.HeartbeatInterval)
	defer heartbeatTimer.Stop()

	currentInterval := cfg.SyncInterval
	paused := false

	for {
		select {
		case <-ctx.Done():
			cfg.Logger.Info("daemon: shutting down")
			return nil

		case <-syncTimer.C:
			nextInterval, nowPaused := runSyncTick(ctx, &cfg, currentInterval, paused, rnd)
			currentInterval, paused = nextInterval, nowPaused
			if cfg.OnTick != nil {
				cfg.OnTick(currentInterval, paused)
			}
			syncTimer.Reset(currentInterval)

		case <-heartbeatTimer.C:
			runHeartbeatTick(ctx, &cfg)
			heartbeatTimer.Reset(cfg.HeartbeatInterval)
		}
	}
}

// runSyncTick performs one sync attempt and decides the interval before
// the next one: reset to the base interval on full success, exponential
// backoff (with jitter, respecting Retry-After) on a recoverable failure,
// or a long paused recheck interval when the device isn't paired or the
// backend rejected its token (section 15: stop automatic retries on
// 401/403 until re-paired).
func runSyncTick(ctx context.Context, cfg *Config, currentInterval time.Duration, paused bool, rnd *rand.Rand) (time.Duration, bool) {
	if paused {
		dc, err := config.LoadDeviceConfig(cfg.ConfigDir)
		if err != nil || !dc.Paired {
			return cfg.PausedRecheckInterval, true
		}
		cfg.Logger.Info("daemon: device is paired again, resuming sync")
		paused = false
	}

	result, err := cfg.Syncer.SyncOnce(ctx)
	switch {
	case errors.Is(err, syncer.ErrNotPaired):
		cfg.Logger.Warn("daemon: device not paired, pausing sync until it is")
		return cfg.PausedRecheckInterval, true

	case errors.Is(err, syncer.ErrUnauthorized):
		cfg.Logger.Error("daemon: backend rejected device token (401/403), pausing sync — re-pair required")
		return cfg.PausedRecheckInterval, true

	case err != nil:
		cfg.Logger.Warn("daemon: sync error: %v", err)
		return withJitter(nextBackoff(currentInterval, cfg.MaxBackoff), rnd), false

	case result.StoppedReason != "":
		cfg.Logger.Warn("daemon: sync stopped early: %s (synced %d, remaining %d)",
			result.StoppedReason, result.Synced, result.Remaining)
		wait := nextBackoff(currentInterval, cfg.MaxBackoff)
		if result.RetryAfter > wait {
			wait = result.RetryAfter
		}
		return withJitter(wait, rnd), false

	default:
		if result.Synced > 0 {
			cfg.Logger.Info("daemon: synced %d item(s)", result.Synced)
		}
		return cfg.SyncInterval, false
	}
}

func runHeartbeatTick(ctx context.Context, cfg *Config) {
	dc, err := config.LoadDeviceConfig(cfg.ConfigDir)
	if err != nil || !dc.Paired {
		return
	}
	if err := cfg.Syncer.Heartbeat(ctx, dc.DeviceID); err != nil {
		cfg.Logger.Warn("daemon: heartbeat failed: %v", err)
	}
}

func lockPath(dataDir string) string {
	return filepath.Join(dataDir, lockFileName)
}
