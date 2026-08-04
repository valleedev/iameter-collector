//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/valleedev/iameter-collector/internal/fsutil"
)

const launchAgentLabel = "com.iameter.collector"

// launchAgentPlist generates the LaunchAgent property list. Built by hand
// (not encoding/xml) because plist's flat alternating <key>/<value>
// structure inside one <dict> doesn't map cleanly onto Go structs.
func launchAgentPlist(binaryPath, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlEscape(launchAgentLabel), xmlEscape(binaryPath), xmlEscape(logPath), xmlEscape(logPath))
}

func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func launchAgentLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Logs", "IAMeter", "daemon.log"), nil
}

type launchdServiceManager struct{}

func newPlatformServiceManager() ServiceManager {
	return launchdServiceManager{}
}

func (launchdServiceManager) Install(binaryPath string) error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return fmt.Errorf("daemon: locate LaunchAgents dir: %w", err)
	}
	logPath, err := launchAgentLogPath()
	if err != nil {
		return fmt.Errorf("daemon: locate log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("daemon: create log dir: %w", err)
	}

	content := launchAgentPlist(binaryPath, logPath)
	if err := fsutil.AtomicWriteFile(plistPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("daemon: write plist: %w", err)
	}

	// Unload first (ignore errors — it may not be loaded yet) so a
	// re-install picks up a changed binary path instead of no-op'ing.
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	if out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("daemon: launchctl load: %w: %s", err, out)
	}
	return nil
}

func (launchdServiceManager) Uninstall() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return fmt.Errorf("daemon: locate LaunchAgents dir: %w", err)
	}
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return nil // idempotent: nothing installed
	}
	_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("daemon: remove plist: %w", err)
	}
	return nil
}

func (launchdServiceManager) Status() (ServiceStatus, error) {
	plistPath, err := launchAgentPath()
	if err != nil {
		return ServiceStatus{}, err
	}
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return ServiceStatus{Detail: "not installed"}, nil
	}
	out, err := exec.Command("launchctl", "list", launchAgentLabel).Output()
	running := err == nil && len(out) > 0
	return ServiceStatus{
		Installed: true,
		Running:   running,
		Detail:    fmt.Sprintf("LaunchAgent installed, running=%v", running),
	}, nil
}
