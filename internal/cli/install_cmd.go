package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/iameter/collector/internal/platform"
	"github.com/iameter/collector/internal/settings"
	"github.com/iameter/collector/internal/version"
)

// cmdInstall wires IA METER into Claude Code's statusLine (section 13/21).
// It assumes the iameter binary is already at its final location — placing
// it there is the job of installers/install.sh|ps1 (Phase 7). Daemon
// service registration (Phase 6) and pairing (Phase 5) are not yet part of
// this command; it reports that plainly rather than pretending to do them.
func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("iameter install", flag.ContinueOnError)
	g := registerGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := g.resolve()

	binPath, err := resolveOwnBinaryPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter install: could not resolve binary path:", err)
		return 1
	}

	settingsPath, err := settings.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter install: could not locate Claude Code settings:", err)
		return 1
	}

	cfg := settings.InstallConfig{
		SettingsPath:   settingsPath,
		BackupPath:     settings.BackupPath(settingsPath),
		ChainStatePath: settings.DefaultChainStatePath(opts.ConfigDir),
		IAMeterCommand: settings.BuildCommand(binPath),
	}

	res, err := settings.Install(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter install: statusLine setup failed:", err)
		return 1
	}

	deviceID, err := ensureDeviceID(opts.ConfigDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "iameter install: could not initialize device id:", err)
		return 1
	}

	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return zeroOr(enc.Encode(map[string]any{
			"binary_path":       binPath,
			"settings_path":     settingsPath,
			"statusline_action": res.Action,
			"device_id":         deviceID,
			"claude_code_found": claudeCodeDetected(),
		}))
	}

	fmt.Printf("IA METER Collector %s\n\n", version.Version)
	fmt.Printf("✓ Sistema detectado: %s %s\n", platform.OS(), platform.Arch())
	fmt.Printf("✓ Binario en uso: %s\n", binPath)
	if claudeCodeDetected() {
		fmt.Println("✓ Claude Code encontrado")
	} else {
		fmt.Println("[WARN] No se detectó Claude Code (~/.claude no existe todavía)")
	}
	switch res.Action {
	case settings.ActionInstalled:
		fmt.Println("✓ StatusLine configurado")
	case settings.ActionChained:
		fmt.Printf("✓ StatusLine configurado (statusLine anterior preservado: %s)\n", res.ChainedFrom.Command)
	case settings.ActionUpdated:
		fmt.Println("✓ StatusLine actualizado (la ruta del binario cambió)")
	case settings.ActionUnchanged:
		fmt.Println("✓ StatusLine ya estaba configurado correctamente")
	}
	fmt.Println("✓ Configuración anterior respaldada en", cfg.BackupPath)
	fmt.Printf("✓ device_id local: %s\n", deviceID)
	fmt.Println("[WARN] Emparejamiento con el backend: pendiente (Phase 5) — ejecuta `iameter pair <CODE>` cuando esté disponible")
	fmt.Println("[WARN] Sincronización en segundo plano: pendiente (Phase 6)")
	fmt.Println()
	fmt.Println("Abre Claude Code y envía un mensaje para obtener el primer dato de consumo,")
	fmt.Println("luego ejecuta `iameter status`.")
	return 0
}

// resolveOwnBinaryPath returns the canonical absolute path to the running
// executable, resolving symlinks so the installed statusLine command
// points at the real file even if invoked through a symlink (e.g. a
// ~/.local/bin/iameter symlink into a versioned install directory).
func resolveOwnBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil // fall back to the unresolved path rather than fail install
	}
	return resolved, nil
}

// claudeCodeDetected is a best-effort check: either Claude Code has a user
// config directory, or its CLI is on PATH. Absence is not fatal — the user
// may install Claude Code after IA METER.
func claudeCodeDetected() bool {
	home, err := os.UserHomeDir()
	if err == nil {
		if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
			return true
		}
	}
	_, err = exec.LookPath("claude")
	return err == nil
}
