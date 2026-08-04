package cli

import (
	"reflect"
	"testing"
)

func TestReorderFlagsFirstMovesFlagsBeforePositional(t *testing.T) {
	got := reorderFlagsFirst([]string{"CM-7X4P2Q", "--config-dir", "/tmp/x", "--json"})
	want := []string{"--config-dir", "/tmp/x", "--json", "CM-7X4P2Q"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReorderFlagsFirstAlreadyOrdered(t *testing.T) {
	got := reorderFlagsFirst([]string{"--json", "CM-7X4P2Q"})
	want := []string{"--json", "CM-7X4P2Q"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReorderFlagsFirstEqualsForm(t *testing.T) {
	got := reorderFlagsFirst([]string{"CM-CODE", "--config-dir=/tmp/x"})
	want := []string{"--config-dir=/tmp/x", "CM-CODE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitArgsCommandFirst(t *testing.T) {
	cmd, rest := splitArgs([]string{"status", "--json"})
	if cmd != "status" || len(rest) != 1 || rest[0] != "--json" {
		t.Errorf("cmd=%q rest=%v", cmd, rest)
	}
}

func TestSplitArgsGlobalFlagBeforeCommand(t *testing.T) {
	cmd, rest := splitArgs([]string{"--config-dir", "/tmp/x", "status"})
	if cmd != "status" {
		t.Errorf("cmd = %q, want status", cmd)
	}
	want := []string{"--config-dir", "/tmp/x"}
	if !reflect.DeepEqual(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}
