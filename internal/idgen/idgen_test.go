package idgen

import (
	"strings"
	"testing"
)

func TestNewPrefix(t *testing.T) {
	id, err := New("snap")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !strings.HasPrefix(id, "snap_") {
		t.Errorf("New(\"snap\") = %q, want snap_ prefix", id)
	}
}

func TestNewUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := New("x")
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("New() produced duplicate: %s", id)
		}
		seen[id] = true
	}
}
