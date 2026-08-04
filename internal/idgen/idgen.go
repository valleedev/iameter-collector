// Package idgen generates short random local identifiers (device ids,
// queue item ids, ...). It is not a cryptographic secret generator — for
// device tokens and other credentials, see internal/credentials (Phase 5).
package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

// New returns "<prefix>_" followed by 13 lowercase base32 characters
// (8 random bytes, no padding) — e.g. New("dev") -> "dev_ch62pvldehacg".
func New(prefix string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("idgen: generate id: %w", err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return prefix + "_" + strings.ToLower(enc), nil
}
