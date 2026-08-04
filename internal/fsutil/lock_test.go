package fsutil

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestFileLockAcquireRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, err := AcquireFileLock(path, time.Second)
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("lock file still exists after Release()")
	}
}

func TestFileLockSecondAcquireBlocksThenTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, err := AcquireFileLock(path, time.Second)
	if err != nil {
		t.Fatalf("first AcquireFileLock() error = %v", err)
	}
	defer lock.Release()

	start := time.Now()
	_, err = AcquireFileLock(path, 200*time.Millisecond)
	if err == nil {
		t.Fatal("second AcquireFileLock() succeeded while first holder is alive, want error")
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Errorf("returned too fast (%v), expected it to respect timeout", time.Since(start))
	}
}

func TestFileLockReclaimsStaleFromDeadProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	// Simulate a lock file left behind by a process that no longer exists:
	// use a PID that's extremely unlikely to be alive.
	deadPID := 999999
	content := strconv.Itoa(deadPID) + "\n" + strconv.FormatInt(time.Now().Unix(), 10) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireFileLock(path, time.Second)
	if err != nil {
		t.Fatalf("AcquireFileLock() should reclaim stale lock from dead pid, got error = %v", err)
	}
	lock.Release()
}

func TestFileLockReclaimsAfterExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	// Lock "held" by our own PID (alive) but acquired long enough ago that
	// it must be considered stale regardless of liveness.
	old := time.Now().Add(-10 * time.Minute).Unix()
	content := strconv.Itoa(os.Getpid()) + "\n" + strconv.FormatInt(old, 10) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireFileLock(path, time.Second)
	if err != nil {
		t.Fatalf("AcquireFileLock() should reclaim expired lock, got error = %v", err)
	}
	lock.Release()
}

func TestFileLockReleaseNilSafe(t *testing.T) {
	var l *FileLock
	if err := l.Release(); err != nil {
		t.Errorf("Release() on nil = %v, want nil", err)
	}
}
