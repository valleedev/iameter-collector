//go:build darwin

package credentials

import (
	"fmt"
	"os/exec"
)

const keychainService = "com.iameter.collector"

// keychainStore shells out to the `security` CLI (present on every macOS
// install) rather than linking Security.framework via CGO, keeping this
// project CGO-free (section 8).
type keychainStore struct{}

func (keychainStore) Name() string     { return "macos-keychain" }
func (keychainStore) IsFallback() bool { return false }

func (keychainStore) Save(key string, value []byte) error {
	// Overwrite any existing entry: delete-then-add, since `security
	// add-generic-password` fails if the item already exists and -U
	// (update) behavior varies across macOS versions.
	_ = exec.Command("security", "delete-generic-password", "-a", key, "-s", keychainService).Run()
	cmd := exec.Command("security", "add-generic-password",
		"-a", key, "-s", keychainService, "-w", string(value), "-U")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("credentials: security add-generic-password: %w: %s", err, out)
	}
	return nil
}

func (keychainStore) Load(key string) ([]byte, error) {
	cmd := exec.Command("security", "find-generic-password", "-a", key, "-s", keychainService, "-w")
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("credentials: security find-generic-password: %w", err)
	}
	return trimTrailingNewline(out), nil
}

func (keychainStore) Delete(key string) error {
	cmd := exec.Command("security", "delete-generic-password", "-a", key, "-s", keychainService)
	_ = cmd.Run() // idempotent: ignore "not found"
	return nil
}

func trimTrailingNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

func platformStore(_ string) (Store, bool) {
	if _, err := exec.LookPath("security"); err != nil {
		return nil, false
	}
	return keychainStore{}, true
}
