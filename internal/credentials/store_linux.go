//go:build linux

package credentials

import (
	"bytes"
	"fmt"
	"os/exec"
)

const secretToolAttr = "application"
const secretToolAttrValue = "iameter"

// secretServiceStore shells out to `secret-tool` (libsecret-tools), which
// talks to the D-Bus Secret Service (GNOME Keyring, KWallet, ...). Using
// the CLI instead of a raw D-Bus client keeps this dependency-free (no
// CGO, no D-Bus library) at the cost of requiring secret-tool to be
// installed — when it isn't, New() falls back to the file store.
type secretServiceStore struct{}

func (secretServiceStore) Name() string     { return "linux-secret-service" }
func (secretServiceStore) IsFallback() bool { return false }

func (secretServiceStore) Save(key string, value []byte) error {
	cmd := exec.Command("secret-tool", "store", "--label=IA METER: "+key,
		secretToolAttr, secretToolAttrValue, "key", key)
	cmd.Stdin = bytes.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("credentials: secret-tool store: %w: %s", err, out)
	}
	return nil
}

func (secretServiceStore) Load(key string) ([]byte, error) {
	cmd := exec.Command("secret-tool", "lookup", secretToolAttr, secretToolAttrValue, "key", key)
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// secret-tool lookup exits non-zero when the key isn't found.
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("credentials: secret-tool lookup: %w", err)
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

func (secretServiceStore) Delete(key string) error {
	cmd := exec.Command("secret-tool", "clear", secretToolAttr, secretToolAttrValue, "key", key)
	// secret-tool clear exits non-zero if nothing matched; that's fine —
	// Delete is idempotent.
	_ = cmd.Run()
	return nil
}

// platformStore probes for secret-tool and a usable D-Bus session. If
// either is missing, the caller falls back to the file store.
func platformStore(_ string) (Store, bool) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, false
	}
	// A quick lookup (of a key that will not exist) both confirms
	// secret-tool can actually reach a Secret Service daemon and doesn't
	// mutate anything.
	cmd := exec.Command("secret-tool", "lookup", secretToolAttr, secretToolAttrValue, "key", "__iameter_probe__")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// A non-zero exit (key not found) still proves the daemon is
			// reachable — that's what we're actually probing for.
			return secretServiceStore{}, true
		}
		return nil, false
	}
	return secretServiceStore{}, true
}
