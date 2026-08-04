//go:build linux

package daemon

import (
	"strings"
	"testing"
)

// These tests only exercise the pure unit-file generator — never real
// systemctl calls — so they run safely in any CI environment regardless
// of whether a systemd --user session exists. Real Install/Uninstall
// against a live systemd --user session was verified manually (see
// IMPLEMENTATION_PLAN.md Phase 6) rather than as an automated test, since
// go test must not depend on live OS session state (mirrors section 26:
// tests must not depend on external/real accounts).

func TestSystemdUnitContainsExecStart(t *testing.T) {
	unit := systemdUnit("/home/user/.local/bin/iameter")
	if !strings.Contains(unit, `ExecStart="/home/user/.local/bin/iameter" daemon`) {
		t.Errorf("unit missing expected ExecStart:\n%s", unit)
	}
	if !strings.Contains(unit, "[Service]") || !strings.Contains(unit, "[Install]") {
		t.Errorf("unit missing expected sections:\n%s", unit)
	}
	if !strings.Contains(unit, "WantedBy=default.target") {
		t.Errorf("unit missing WantedBy:\n%s", unit)
	}
}

func TestSystemdUnitQuotesSpacedPath(t *testing.T) {
	unit := systemdUnit("/home/user/My Apps/iameter")
	if !strings.Contains(unit, `ExecStart="/home/user/My Apps/iameter" daemon`) {
		t.Errorf("unit did not quote spaced path:\n%s", unit)
	}
}

func TestQuoteUnitPathNoSpaces(t *testing.T) {
	got := quoteUnitPath("/usr/local/bin/iameter")
	if got != `"/usr/local/bin/iameter"` {
		t.Errorf("quoteUnitPath() = %q, want quoted", got)
	}
}

func TestQuoteUnitPathEscapesEmbeddedQuotes(t *testing.T) {
	got := quoteUnitPath(`/home/user/weird"path/iameter`)
	if !strings.Contains(got, `\"`) {
		t.Errorf("quoteUnitPath() = %q, want escaped embedded quote", got)
	}
}
