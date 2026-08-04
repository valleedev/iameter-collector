// Package providers defines the extensibility contract for usage sources.
// The MVP ships one implementation (internal/providers/claude), but the
// queue, syncer, pairing, credential storage, daemon, and installers never
// import it directly — they only depend on model.RateLimits and
// model.UsageSnapshot, so adding a second provider later (OpenAI, Gemini,
// Copilot, ...) means adding one new package under providers/, not
// reworking the rest of the collector.
package providers

import (
	"io"

	"github.com/valleedev/iameter-collector/internal/model"
)

// UsageProvider parses a provider's raw usage payload into the whitelisted
// RateLimits shape. Parse must never return data for fields outside that
// whitelist — whatever it reads from r, only used_percentage/resets_at for
// five_hour/seven_day ever leaves this boundary.
type UsageProvider interface {
	// Name identifies the provider, e.g. "claude". Used as the `provider`
	// field on outgoing snapshots.
	Name() string

	// Parse reads and validates a provider payload from r. It returns a
	// non-nil error only for structurally invalid input (malformed JSON,
	// empty input). A window with an invalid/out-of-range/wrong-typed
	// value is dropped (treated as absent) rather than failing the whole
	// parse — absence is never invented as a fake zero value.
	Parse(r io.Reader) (*model.RateLimits, error)
}
