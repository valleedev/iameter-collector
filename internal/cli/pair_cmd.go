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
	"github.com/iameter/collector/internal/device"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/pairing"
	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/version"
)

const pairTimeout = 15 * time.Second

// cmdPair implements section 16: exchange a one-time pairing code for a
// device token, store it in the platform credential store, and mark the
// device paired locally. The pairing code itself is never persisted.
func cmdPair(args []string) int {
	fs := flag.NewFlagSet("iameter pair", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(reorderFlagsFirst(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: iameter pair <PAIRING_CODE>")
		return 2
	}
	code := fs.Arg(0)
	opts := g.resolve()

	result, creds, err := performPairing(opts, code)
	if err != nil {
		if errors.Is(err, errAlreadyPairedLocally) {
			fmt.Println("Este dispositivo ya está emparejado. Ejecuta `iameter unpair` primero si quieres volver a emparejarlo.")
			return 1
		}
		fmt.Fprintln(os.Stderr, "iameter pair:", pairErrorMessage(err))
		return 1
	}

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(map[string]any{
			"device_id":           result.DeviceID,
			"user_id":             result.UserID,
			"credential_store":    creds.Name(),
			"credential_fallback": creds.IsFallback(),
		}))
	}

	fmt.Println("✓ Dispositivo emparejado correctamente")
	fmt.Println("  device_id:", result.DeviceID)
	if creds.IsFallback() {
		fmt.Println("[WARN] No se encontró un almacén de credenciales nativo del sistema.")
		fmt.Println("       El token se guardó en un archivo local con permisos restrictivos (" + creds.Name() + ").")
	}
	fmt.Println()
	fmt.Println("Ejecuta `iameter sync` o `iameter daemon` para empezar a sincronizar.")
	return 0
}

// errAlreadyPairedLocally is a client-side short-circuit: this collector
// already has a stored device token, so a re-pair must go through
// `iameter unpair` first rather than silently overwriting credentials.
var errAlreadyPairedLocally = errors.New("device already paired locally")

// performPairing runs the full pairing exchange and persists the result
// (device config + credential store). Shared by `iameter pair` and
// `iameter install --pair CODE`.
func performPairing(opts config.Options, code string) (*pairing.Result, credentials.Store, error) {
	dc, err := config.LoadDeviceConfig(opts.ConfigDir)
	if err != nil {
		return nil, nil, err
	}
	if dc.Paired {
		return nil, nil, errAlreadyPairedLocally
	}

	deviceName := dc.DeviceName
	if deviceName == "" {
		deviceName = device.DefaultName()
	}
	info := pairing.DeviceInfo{
		Name:             deviceName,
		OS:               platform.OS(),
		Arch:             platform.Arch(),
		CollectorVersion: version.Version,
	}

	client := httpclient.New(opts.APIBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), pairTimeout)
	defer cancel()

	result, err := pairing.Pair(ctx, client, code, info)
	if err != nil {
		return nil, nil, err
	}

	creds := credentials.New(opts.DataDir)
	if err := creds.Save("device_token", []byte(result.DeviceToken)); err != nil {
		return nil, nil, fmt.Errorf("could not store device token: %w", err)
	}

	dc.DeviceID = result.DeviceID // backend's device_id supersedes the local one
	dc.DeviceName = deviceName
	dc.Paired = true
	dc.UserID = result.UserID
	dc.PairedAt = time.Now().UTC().Format(time.RFC3339)
	if err := config.SaveDeviceConfig(opts.ConfigDir, dc); err != nil {
		return nil, nil, fmt.Errorf("could not save device config: %w", err)
	}

	return result, creds, nil
}

func pairErrorMessage(err error) string {
	switch {
	case errors.Is(err, pairing.ErrInvalidFormat):
		return "código de emparejamiento con formato inválido"
	case errors.Is(err, pairing.ErrExpiredOrNotFound):
		return "código de emparejamiento expirado o no encontrado"
	case errors.Is(err, pairing.ErrAlreadyUsed):
		return "código de emparejamiento ya utilizado"
	case errors.Is(err, pairing.ErrAlreadyPaired):
		return "el dispositivo ya está emparejado en el backend"
	case errors.Is(err, pairing.ErrInvalidResponse):
		return "el backend devolvió una respuesta inválida: " + err.Error()
	case errors.Is(err, pairing.ErrServer):
		return "error del backend: " + err.Error()
	default:
		return "error de red: " + err.Error()
	}
}
