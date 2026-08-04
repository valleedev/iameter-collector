package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/iameter/collector/internal/daemon"
	"github.com/iameter/collector/internal/settings"
	"github.com/iameter/collector/internal/version"
)

// cmdUninstall restores Claude Code's statusLine to its pre-IAMETER state
// (section 13/21) and unregisters the background daemon service (section
// 20). It intentionally does not remove the local device config, queue, or
// credentials — those are the concern of `iameter unpair`.
func cmdUninstall(args []string) int {
	fs := flag.NewFlagSet("iameter uninstall", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	settingsPath, err := settings.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter uninstall: could not locate Claude Code settings:", err)
		return 1
	}

	cfg := settings.UninstallConfig{
		SettingsPath:   settingsPath,
		ChainStatePath: settings.DefaultChainStatePath(opts.ConfigDir),
	}

	res, err := settings.Uninstall(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter uninstall: statusLine restore failed:", err)
		return 1
	}

	svcErr := daemon.NewServiceManager().Uninstall()

	if opts.JSON {
		out := map[string]any{
			"statusline_action":   res.Action,
			"collector_version":   version.Version,
			"daemon_unregistered": svcErr == nil,
		}
		if svcErr != nil {
			out["daemon_error"] = svcErr.Error()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(out))
	}

	fmt.Printf("IA METER Collector %s — uninstall\n\n", version.Version)
	switch res.Action {
	case settings.UninstallRestoredChain:
		fmt.Println("✓ StatusLine anterior restaurado")
	case settings.UninstallRemoved:
		fmt.Println("✓ StatusLine de IA METER eliminado")
	case settings.UninstallNotOurs:
		fmt.Println("StatusLine actual no pertenece a IA METER, no se modificó")
	case settings.UninstallNoop:
		fmt.Println("IA METER no estaba instalado en el statusLine (nada que hacer)")
	}
	if svcErr == nil {
		fmt.Println("✓ Servicio en segundo plano eliminado")
	} else {
		fmt.Println("[WARN] No se pudo eliminar el servicio en segundo plano:", svcErr)
	}
	fmt.Println()
	fmt.Println("El device_id local y la cola (si existe) se conservan.")
	fmt.Println("Ejecuta `iameter unpair` para eliminar credenciales locales.")
	return 0
}
