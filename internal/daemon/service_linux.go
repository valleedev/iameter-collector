//go:build linux

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/valleedev/iameter-collector/internal/fsutil"
)

const systemdUnitName = "iameter.service"

// systemdUnit generates the systemd --user unit file content. Restart=
// on-failure with a short RestartSec gives basic resilience without
// needing our own supervisor logic; the daemon's own single-instance lock
// (section 15) prevents overlapping restarts from racing.
func systemdUnit(binaryPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\n")
	fmt.Fprintf(&b, "Description=IA METER Collector background sync daemon\n")
	fmt.Fprintf(&b, "After=network-online.target\n")
	fmt.Fprintf(&b, "\n[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s daemon\n", quoteUnitPath(binaryPath))
	fmt.Fprintf(&b, "Restart=on-failure\n")
	fmt.Fprintf(&b, "RestartSec=10\n")
	fmt.Fprintf(&b, "\n[Install]\n")
	fmt.Fprintf(&b, "WantedBy=default.target\n")
	return b.String()
}

// quoteUnitPath always wraps the path in double quotes (escaping any
// embedded ones), per systemd unit file quoting rules. Always quoting
// rather than only when spaces are detected avoids under-quoting a path
// that contains other word-splitting-sensitive characters without spaces
// (section 31: paths with spaces/Unicode).
func quoteUnitPath(path string) string {
	return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
}

func systemdUnitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user"), nil
}

type systemdServiceManager struct{}

func newPlatformServiceManager() ServiceManager {
	return systemdServiceManager{}
}

func (systemdServiceManager) available() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemd not available: `systemctl` not found on PATH — " +
			"start `iameter daemon` manually (e.g. via a cron @reboot entry) instead")
	}
	return nil
}

func (s systemdServiceManager) Install(binaryPath string) error {
	if err := s.available(); err != nil {
		return err
	}
	dir, err := systemdUnitDir()
	if err != nil {
		return fmt.Errorf("daemon: locate systemd user dir: %w", err)
	}
	unitPath := filepath.Join(dir, systemdUnitName)
	if err := fsutil.AtomicWriteFile(unitPath, []byte(systemdUnit(binaryPath)), 0o600); err != nil {
		return fmt.Errorf("daemon: write unit file: %w", err)
	}

	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon: systemctl daemon-reload: %w: %s", err, out)
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", systemdUnitName).CombinedOutput(); err != nil {
		return fmt.Errorf("daemon: systemctl enable --now: %w: %s", err, out)
	}
	return nil
}

func (s systemdServiceManager) Uninstall() error {
	if err := s.available(); err != nil {
		return nil // nothing to uninstall if systemd was never usable
	}
	// disable --now on a unit that was never enabled just fails harmlessly;
	// ignore the error to keep Uninstall idempotent.
	_ = exec.Command("systemctl", "--user", "disable", "--now", systemdUnitName).Run()

	dir, err := systemdUnitDir()
	if err != nil {
		return fmt.Errorf("daemon: locate systemd user dir: %w", err)
	}
	unitPath := filepath.Join(dir, systemdUnitName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("daemon: remove unit file: %w", err)
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func (s systemdServiceManager) Status() (ServiceStatus, error) {
	if err := s.available(); err != nil {
		return ServiceStatus{Detail: err.Error()}, nil
	}
	dir, err := systemdUnitDir()
	if err != nil {
		return ServiceStatus{}, err
	}
	unitPath := filepath.Join(dir, systemdUnitName)
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return ServiceStatus{Detail: "not installed"}, nil
	}

	activeOut, _ := exec.Command("systemctl", "--user", "is-active", systemdUnitName).Output()
	active := strings.TrimSpace(string(activeOut)) == "active"

	enabledOut, _ := exec.Command("systemctl", "--user", "is-enabled", systemdUnitName).Output()
	enabled := strings.TrimSpace(string(enabledOut)) == "enabled"

	return ServiceStatus{
		Installed: enabled,
		Running:   active,
		Detail:    fmt.Sprintf("systemd --user: enabled=%v active=%v", enabled, active),
	}, nil
}
