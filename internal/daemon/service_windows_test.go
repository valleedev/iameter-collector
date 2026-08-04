//go:build windows

package daemon

import (
	"strings"
	"testing"
)

func TestSchtasksActionQuotesPath(t *testing.T) {
	got := schtasksAction(`C:\Program Files\IAMeter\iameter.exe`)
	want := `"C:\Program Files\IAMeter\iameter.exe" daemon`
	if got != want {
		t.Errorf("schtasksAction() = %q, want %q", got, want)
	}
}

func TestSchtasksActionEscapesEmbeddedQuotes(t *testing.T) {
	got := schtasksAction(`C:\weird"path\iameter.exe`)
	if !strings.Contains(got, `""`) {
		t.Errorf("schtasksAction() = %q, want escaped embedded quote", got)
	}
}
