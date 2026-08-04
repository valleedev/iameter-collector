package capture

import (
	"strings"
	"testing"
)

func TestReadLimitedNormal(t *testing.T) {
	data, err := ReadLimited(strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatalf("ReadLimited() error = %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Errorf("data = %q", data)
	}
}

func TestReadLimitedExactlyAtLimit(t *testing.T) {
	in := strings.Repeat("a", MaxInputBytes)
	data, err := ReadLimited(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadLimited() error = %v, want nil at exactly the limit", err)
	}
	if len(data) != MaxInputBytes {
		t.Errorf("len(data) = %d, want %d", len(data), MaxInputBytes)
	}
}

func TestReadLimitedOverLimit(t *testing.T) {
	in := strings.Repeat("a", MaxInputBytes+1)
	_, err := ReadLimited(strings.NewReader(in))
	if err == nil {
		t.Fatal("ReadLimited() error = nil, want error for oversized input")
	}
}

func TestReadLimitedEmpty(t *testing.T) {
	data, err := ReadLimited(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ReadLimited() error = %v", err)
	}
	if len(data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(data))
	}
}
