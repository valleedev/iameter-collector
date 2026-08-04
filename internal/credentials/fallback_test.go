package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileStoreSaveLoadRoundTrip(t *testing.T) {
	s := newFileStore(t.TempDir())
	if err := s.Save("device_token", []byte("secret-value")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load("device_token")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got) != "secret-value" {
		t.Errorf("Load() = %q, want secret-value", got)
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	s := newFileStore(t.TempDir())
	_, err := s.Load("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Load() error = %v, want ErrNotFound", err)
	}
}

func TestFileStoreDeleteIdempotent(t *testing.T) {
	s := newFileStore(t.TempDir())
	if err := s.Delete("never-existed"); err != nil {
		t.Errorf("Delete() on missing key error = %v, want nil", err)
	}
	s.Save("k", []byte("v"))
	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := s.Load("k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load() after Delete() error = %v, want ErrNotFound", err)
	}
	if err := s.Delete("k"); err != nil {
		t.Errorf("second Delete() error = %v, want nil (idempotent)", err)
	}
}

func TestFileStorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits don't apply on windows")
	}
	dir := t.TempDir()
	s := newFileStore(dir)
	s.Save("device_token", []byte("secret-value"))

	path := filepath.Join(dir, "credentials", "device_token.cred")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestFileStoreRejectsPathTraversalKey(t *testing.T) {
	s := newFileStore(t.TempDir())
	for _, badKey := range []string{"../escape", "a/../../b", "/etc/passwd", "sub/dir"} {
		if err := s.Save(badKey, []byte("x")); err == nil {
			t.Errorf("Save(%q) error = nil, want error (path traversal guard)", badKey)
		}
	}
}

func TestNewReturnsAStore(t *testing.T) {
	s := New(t.TempDir())
	if s == nil {
		t.Fatal("New() = nil")
	}
	if s.Name() == "" {
		t.Error("Name() is empty")
	}
	// Whatever backend New() picked, it must actually work end-to-end.
	if err := s.Save("probe", []byte("value")); err != nil {
		t.Fatalf("Save() error = %v (store = %s)", err, s.Name())
	}
	defer s.Delete("probe")
	got, err := s.Load("probe")
	if err != nil {
		t.Fatalf("Load() error = %v (store = %s)", err, s.Name())
	}
	if string(got) != "value" {
		t.Errorf("Load() = %q, want value", got)
	}
}
