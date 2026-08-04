package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iameter/collector/internal/mockserver"
)

const mockServerShutdownGrace = 3 * time.Second

// cmdMockServer runs the local development backend (section 30). It is
// intentionally not one of the 10 primary commands (section 10) — it's a
// development utility, never presented as a production endpoint.
func cmdMockServer(args []string) int {
	fs := flag.NewFlagSet("iameter mock-server", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	addr := fs.String("addr", "127.0.0.1:8787", "address to listen on")
	pairingCode := fs.String("pairing-code", "", "preset pairing code (random one generated if omitted)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_ = g.resolve()

	var ms *mockserver.Server
	code := *pairingCode
	if code != "" {
		ms = mockserver.New(code)
	} else {
		ms = mockserver.New()
		code = ms.NewPairingCode()
	}
	ms.Logger = log.New(os.Stdout, "", log.LstdFlags)

	fmt.Println("IA METER mock backend (DEVELOPMENT ONLY — not a real server)")
	fmt.Println()
	fmt.Printf("  Listening on:  http://%s\n", *addr)
	fmt.Printf("  Pairing code:  %s\n", code)
	fmt.Println()
	fmt.Println("Try:")
	fmt.Printf("  iameter pair %s --api-base-url http://%s\n", code, *addr)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	srv := &http.Server{Addr: *addr, Handler: ms.Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "iameter mock-server:", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), mockServerShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintln(os.Stderr, "iameter mock-server: shutdown:", err)
			return 1
		}
		fmt.Println("\nmock-server stopped")
	}
	return 0
}
