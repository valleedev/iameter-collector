package statusline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/iameter/collector/internal/platform"
)

// ChainTimeout bounds how long a preserved third-party statusLine command
// is allowed to run. Claude Code calls the statusLine command frequently
// (section: "runs frequently during active sessions"), so a hung child
// process must not hang IA METER indefinitely.
const ChainTimeout = 3 * time.Second

// waitDelay bounds how long Run() keeps waiting for output-pipe I/O after
// the command is canceled. Without this, a grandchild process the shell
// forked (e.g. `sh -c "sleep 30"` forking `sleep`) can keep the stdout
// pipe open after the shell itself is killed, and cmd.Run() would block
// until that grandchild exits on its own — defeating ChainTimeout
// entirely. killProcessGroup (below) also directly kills that grandchild
// on platforms where process groups are available; waitDelay is the
// cross-platform backstop.
const waitDelay = 1 * time.Second

// RunChained executes a previously-configured third-party statusLine
// command (section 13), feeding it the same stdin JSON Claude Code gave
// IA METER, and returns its stdout so IA METER can print that instead of
// its own render — preserving the previous tool's visual output exactly.
//
// The child's environment is filtered to strip any IAMETER_-prefixed
// variables so a future credential-bearing env var (none exist yet) could
// never reach it (section 13: "no expone tokens al proceso hijo").
func RunChained(ctx context.Context, command string, stdin []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ChainTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if platform.IsWindows() {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Env = filterEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.WaitDelay = waitDelay
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("statusline: chained command failed: %w", err)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func filterEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "IAMETER_") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}
