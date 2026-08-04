package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/daemon"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/logging"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/syncer"
)

// cmdDaemon implements section 15: run the background sync loop in the
// foreground until signaled to stop. Service managers (systemd --user,
// LaunchAgent, Scheduled Task — registered by `iameter install`, section
// 20) are what actually keep this running long-term; this command itself
// does no self-daemonization/forking.
func cmdDaemon(args []string) int {
	fs := flag.NewFlagSet("iameter daemon", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()
	logger := logging.Default(opts.LogLevel)

	q, err := queue.Open(opts.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter daemon: open queue:", err)
		return 1
	}
	creds := credentials.New(opts.DataDir)
	client := httpclient.New(opts.APIBaseURL)
	s := syncer.New(q, client, creds)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := daemon.Config{
		Syncer:    s,
		ConfigDir: opts.ConfigDir,
		DataDir:   opts.DataDir,
		Logger:    logger,
	}

	err = daemon.Run(ctx, cfg)
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		fmt.Fprintln(os.Stderr, "iameter daemon: another instance is already running")
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter daemon:", err)
		return 1
	}
	return 0
}
