//go:build windows

package daemon

import (
	"fmt"
	"os/exec"
	"strings"
)

const taskName = "IAMeterCollectorDaemon"

// schtasksAction builds the /TR (task run) command, quoting the binary
// path so a space-containing path (very common on Windows —
// "%LOCALAPPDATA%\IAMeter\bin\iameter.exe" is fine, but a user-chosen
// install dir under "Program Files" or similar would break unquoted)
// works correctly (section 31).
func schtasksAction(binaryPath string) string {
	return fmt.Sprintf(`"%s" daemon`, strings.ReplaceAll(binaryPath, `"`, `""`))
}

type schtasksServiceManager struct{}

func newPlatformServiceManager() ServiceManager {
	return schtasksServiceManager{}
}

// Install registers a Scheduled Task that runs at logon under the current
// user — no administrator privileges required (section 20: "no utilizar
// un servicio que requiera administrador para el MVP").
func (schtasksServiceManager) Install(binaryPath string) error {
	action := schtasksAction(binaryPath)
	cmd := exec.Command("schtasks", "/Create",
		"/SC", "ONLOGON",
		"/TN", taskName,
		"/TR", action,
		"/RL", "LIMITED",
		"/F", // overwrite if it already exists: idempotent
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daemon: schtasks /Create: %w: %s", err, out)
	}
	// Also start it now, so the user doesn't have to log out/in for the
	// daemon to begin syncing.
	_ = exec.Command("schtasks", "/Run", "/TN", taskName).Run()
	return nil
}

func (schtasksServiceManager) Uninstall() error {
	cmd := exec.Command("schtasks", "/Delete", "/TN", taskName, "/F")
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "cannot find") {
		return fmt.Errorf("daemon: schtasks /Delete: %w: %s", err, out)
	}
	return nil // idempotent: "cannot find" means already uninstalled
}

func (schtasksServiceManager) Status() (ServiceStatus, error) {
	out, err := exec.Command("schtasks", "/Query", "/TN", taskName, "/FO", "LIST").Output()
	if err != nil {
		return ServiceStatus{Detail: "not installed"}, nil
	}
	text := string(out)
	running := strings.Contains(text, "Running")
	return ServiceStatus{
		Installed: true,
		Running:   running,
		Detail:    "Scheduled Task installed",
	}, nil
}
