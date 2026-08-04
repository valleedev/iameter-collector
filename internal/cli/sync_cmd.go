package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/valleedev/iameter-collector/internal/config"
	"github.com/valleedev/iameter-collector/internal/credentials"
	"github.com/valleedev/iameter-collector/internal/httpclient"
	"github.com/valleedev/iameter-collector/internal/queue"
	"github.com/valleedev/iameter-collector/internal/syncer"
)

const syncTimeout = 30 * time.Second

// cmdSync implements section 15: attempt one immediate sync pass and
// exit — no retry loop, no backoff (that's the daemon's job, Phase 6).
func cmdSync(args []string) int {
	fs := flag.NewFlagSet("iameter sync", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	dc, err := config.LoadDeviceConfig(opts.ConfigDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter sync: load device config:", err)
		return 1
	}
	if !dc.Paired {
		fmt.Println("Dispositivo no emparejado. Ejecuta `iameter pair <CODE>` primero.")
		return 1
	}

	q, err := queue.Open(opts.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter sync: open queue:", err)
		return 1
	}

	creds := credentials.New(opts.DataDir)
	client := httpclient.New(opts.APIBaseURL)
	s := syncer.New(q, client, creds)

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	result, syncErr := s.SyncOnce(ctx)

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		out := map[string]any{
			"synced":         result.Synced,
			"remaining":      result.Remaining,
			"stopped_reason": result.StoppedReason,
		}
		if syncErr != nil {
			out["error"] = syncErr.Error()
		}
		code := zeroOr(enc.Encode(out))
		if syncErr != nil {
			return 1
		}
		return code
	}

	switch {
	case errors.Is(syncErr, syncer.ErrNotPaired):
		fmt.Println("Dispositivo no emparejado. Ejecuta `iameter pair <CODE>` primero.")
		return 1
	case errors.Is(syncErr, syncer.ErrUnauthorized):
		fmt.Println("[ERROR] El backend rechazó el token del dispositivo (401/403).")
		fmt.Println("        Ejecuta `iameter unpair` y `iameter pair <CODE>` para volver a emparejar.")
		return 1
	case syncErr != nil:
		fmt.Fprintln(os.Stderr, "iameter sync:", syncErr)
		return 1
	}

	fmt.Printf("Sincronizados: %d, pendientes: %d\n", result.Synced, result.Remaining)
	if result.StoppedReason != "" {
		fmt.Printf("[WARN] Sincronización detenida: %s\n", result.StoppedReason)
		if result.RetryAfter > 0 {
			fmt.Printf("       El backend pide reintentar en %s\n", result.RetryAfter)
		}
		return 1
	}
	return 0
}
