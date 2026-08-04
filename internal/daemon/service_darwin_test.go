//go:build darwin

package daemon

import (
	"strings"
	"testing"
)

func TestLaunchAgentPlistWellFormed(t *testing.T) {
	p := launchAgentPlist("/opt/iameter/iameter", "/home/user/Library/Logs/IAMeter/daemon.log")
	if !strings.Contains(p, "<key>Label</key>") || !strings.Contains(p, launchAgentLabel) {
		t.Errorf("plist missing Label:\n%s", p)
	}
	if !strings.Contains(p, "<string>/opt/iameter/iameter</string>") {
		t.Errorf("plist missing binary path:\n%s", p)
	}
	if !strings.Contains(p, "<string>daemon</string>") {
		t.Errorf("plist missing daemon argument:\n%s", p)
	}
	if !strings.Contains(p, "<true/>") {
		t.Errorf("plist missing RunAtLoad/KeepAlive:\n%s", p)
	}
}

func TestXMLEscape(t *testing.T) {
	got := xmlEscape(`a & b < c > d "e"`)
	want := `a &amp; b &lt; c &gt; d &quot;e&quot;`
	if got != want {
		t.Errorf("xmlEscape() = %q, want %q", got, want)
	}
}
