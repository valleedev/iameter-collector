//go:build !windows

package statusline

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group so a timeout can
// kill the whole tree (the shell *and* whatever it forked, e.g.
// `sh -c "sleep 30"` forks a `sleep` grandchild that would otherwise be
// orphaned and keep the stdout pipe open after the shell itself is killed).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the negative PID, i.e. the whole
// process group created by setProcessGroup.
func killProcessGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
