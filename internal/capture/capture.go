// Package capture reads size-bounded input from Claude Code's statusLine
// stdin before handing it to a provider parser, so a misbehaving or
// malicious hook can't feed iameter unbounded input.
package capture

import (
	"fmt"
	"io"
)

// MaxInputBytes caps statusLine JSON input. Real payloads are a few KB;
// 1 MiB is generous headroom while still bounding worst-case memory use.
const MaxInputBytes = 1 << 20 // 1 MiB

// ReadLimited reads at most MaxInputBytes from r. If the input is larger,
// it returns an error instead of silently truncating (truncated JSON would
// just fail to parse anyway, but this gives a clearer error message).
func ReadLimited(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, MaxInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("capture: read input: %w", err)
	}
	if len(data) > MaxInputBytes {
		return nil, fmt.Errorf("capture: input exceeds maximum size of %d bytes", MaxInputBytes)
	}
	return data, nil
}
