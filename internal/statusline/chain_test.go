package statusline

import (
	"context"
	"strings"
	"testing"

	"github.com/valleedev/iameter-collector/internal/platform"
)

func TestRunChainedEchoesStdin(t *testing.T) {
	if platform.IsWindows() {
		t.Skip("shell command differs on windows")
	}
	out, err := RunChained(context.Background(), `cat`, []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("RunChained() error = %v", err)
	}
	if out != `{"a":1}` {
		t.Errorf("out = %q", out)
	}
}

func TestRunChainedPreservesChildOutput(t *testing.T) {
	if platform.IsWindows() {
		t.Skip("shell command differs on windows")
	}
	out, err := RunChained(context.Background(), `echo "previous tool output"`, []byte(`{}`))
	if err != nil {
		t.Fatalf("RunChained() error = %v", err)
	}
	if out != "previous tool output" {
		t.Errorf("out = %q", out)
	}
}

func TestRunChainedTimesOutOnHang(t *testing.T) {
	if platform.IsWindows() {
		t.Skip("shell command differs on windows")
	}
	_, err := RunChained(context.Background(), `sleep 30`, nil)
	if err == nil {
		t.Fatal("RunChained() error = nil, want timeout error")
	}
}

func TestRunChainedFailingCommand(t *testing.T) {
	if platform.IsWindows() {
		t.Skip("shell command differs on windows")
	}
	_, err := RunChained(context.Background(), `exit 1`, nil)
	if err == nil {
		t.Fatal("RunChained() error = nil, want error for nonzero exit")
	}
}

func TestFilterEnvStripsIAMeterVars(t *testing.T) {
	env := []string{"PATH=/usr/bin", "IAMETER_DEVICE_TOKEN=secret", "HOME=/home/x"}
	got := filterEnv(env)
	for _, kv := range got {
		if strings.HasPrefix(kv, "IAMETER_") {
			t.Errorf("filterEnv() leaked %q", kv)
		}
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
}
