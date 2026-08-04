package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/valleedev/iameter-collector/internal/config"
	"github.com/valleedev/iameter-collector/internal/credentials"
	"github.com/valleedev/iameter-collector/internal/device"
)

// cmdUnpair removes local pairing credentials (section: "unpair"). It
// deletes the stored device token and clears the paired state, generating
// a fresh local device_id (the old one belonged to the now-defunct backend
// pairing — see internal/device's doc comment). It does not touch the
// queue or Claude Code's statusLine configuration.
func cmdUnpair(args []string) int {
	fs := flag.NewFlagSet("iameter unpair", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	dc, err := config.LoadDeviceConfig(opts.ConfigDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter unpair: load device config:", err)
		return 1
	}
	if !dc.Paired {
		fmt.Println("El dispositivo no estaba emparejado (nada que hacer).")
		return 0
	}

	creds := credentials.New(opts.DataDir)
	if err := creds.Delete("device_token"); err != nil {
		fmt.Fprintln(os.Stderr, "iameter unpair: could not delete device token:", err)
		return 1
	}

	newID, err := device.NewID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter unpair: could not generate new device id:", err)
		return 1
	}
	dc.DeviceID = newID
	dc.Paired = false
	dc.UserID = ""
	dc.PairedAt = ""
	if err := config.SaveDeviceConfig(opts.ConfigDir, dc); err != nil {
		fmt.Fprintln(os.Stderr, "iameter unpair: could not save device config:", err)
		return 1
	}

	fmt.Println("✓ Credenciales de emparejamiento eliminadas")
	return 0
}
