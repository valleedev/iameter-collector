package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FileLock is a simple cross-process, cross-platform advisory lock based on
// exclusive file creation (O_CREATE|O_EXCL), not syscall.Flock — Flock has
// no portable Windows equivalent without cgo, and this project avoids CGO
// (section 8, section 27 risk #4). The lock file holds the holder's PID and
// an acquisition timestamp so a stale lock left behind by a crashed process
// can be detected and reclaimed.
type FileLock struct {
	path string
	file *os.File
}

// staleAfter is how long a lock is considered abandoned if its holder
// process no longer exists (checked via signal 0 on Unix; on Windows we
// fall back to the time-based check only, since PID liveness checks differ).
const staleAfter = 2 * time.Minute

// AcquireFileLock attempts to acquire the lock at path, retrying briefly if
// it's held by a live, non-stale process. It returns an error if the lock
// cannot be acquired within timeout.
func AcquireFileLock(path string, timeout time.Duration) (*FileLock, error) {
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n%d\n", os.Getpid(), time.Now().Unix())
			return &FileLock{path: path, file: f}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("fsutil: create lock file: %w", err)
		}
		if reclaimIfStale(path) {
			continue // stale lock removed, retry immediately
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("fsutil: lock %s held by another process (timeout)", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Release removes the lock file. Safe to call once; subsequent calls are
// no-ops.
func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.file.Close()
	err := os.Remove(l.path)
	l.file = nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fsutil: release lock: %w", err)
	}
	return nil
}

// reclaimIfStale removes the lock file at path if it was written by a
// process that's no longer running (Unix) or is older than staleAfter
// (fallback used on all platforms when PID liveness can't be checked).
// Returns true if it removed the file.
func reclaimIfStale(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return false
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	ts, _ := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	acquired := time.Unix(ts, 0)

	stale := time.Since(acquired) >= staleAfter || pid <= 0 || !processAlive(pid)
	if !stale {
		return false
	}
	_ = os.Remove(path)
	return true
}

// lockFileDir ensures the parent directory of a lock path exists.
func EnsureLockDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}
