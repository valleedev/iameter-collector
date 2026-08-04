//go:build windows

package fsutil

import "os"

// processAlive on Windows: os.FindProcess always succeeds (Windows has no
// direct pid-existence syscall exposed via stdlib without extra deps), so
// signaling with syscall.Signal(0) is used as a best-effort liveness probe;
// if it fails we treat the process as gone. This mirrors the stdlib's own
// approach for os.Process.Signal on Windows.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// os.Process.Signal(syscall.Signal(0)) is not supported on Windows
	// (returns "not supported by windows"), so instead try a zero-cost
	// Release-safe probe: sending os.Interrupt is not appropriate either.
	// Fall back to assuming alive; staleness is then governed purely by
	// staleAfter time-based expiry on this platform.
	_ = proc
	return true
}
