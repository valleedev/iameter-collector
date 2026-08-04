package device

import (
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if !strings.HasPrefix(id, "dev_") {
		t.Errorf("NewID() = %q, want dev_ prefix", id)
	}
	if len(id) < 8 {
		t.Errorf("NewID() = %q, too short", id)
	}
}

func TestNewIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		if seen[id] {
			t.Fatalf("NewID() produced duplicate: %s", id)
		}
		seen[id] = true
	}
}

func TestDefaultName(t *testing.T) {
	name := DefaultName()
	if strings.TrimSpace(name) == "" {
		t.Error("DefaultName() returned empty string")
	}
}
