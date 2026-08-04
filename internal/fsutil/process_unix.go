//go:build !windows

package fsutil

import (
	"os"
	"syscall"
)

// processAlive reports whether pid refers to a running process, using
// signal 0 (no-op signal delivery used purely to probe existence/permission).
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
