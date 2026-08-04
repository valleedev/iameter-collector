package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/iameter/collector/internal/settings"
	"github.com/iameter/collector/internal/version"
)

// cmdUninstall restores Claude Code's statusLine to its pre-IAMETER state
// (section 13/21). It intentionally does not remove the local device
// config, queue, or credentials — those are the concern of `iameter
// unpair` (Phase 5) and daemon service removal (Phase 6), which is not
// wired in here yet.
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

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(map[string]any{
			"statusline_action": res.Action,
			"collector_version": version.Version,
		}))
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
	fmt.Println()
	fmt.Println("El device_id local y la cola (si existe) se conservan.")
	fmt.Println("Ejecuta `iameter unpair` para eliminar credenciales locales (Phase 5).")
	return 0
}
