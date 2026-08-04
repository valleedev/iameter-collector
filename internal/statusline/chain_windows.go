//go:build windows

package statusline

import "os/exec"

// On Windows, cmd.WaitDelay alone (set in chain.go) bounds Run()'s wait
// even if a grandchild process keeps a pipe open; there is no direct
// stdlib equivalent to a POSIX process-group kill without extra
// dependencies, which this project avoids (section 8: no CGO, minimal
// deps).
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error { return cmd.Process.Kill() }
