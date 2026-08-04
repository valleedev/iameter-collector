// Package claude implements providers.UsageProvider for Claude Code's
// statusLine JSON. It extracts exactly the whitelisted fields documented in
// https://code.claude.com/docs/en/statusline (rate_limits.five_hour and
// rate_limits.seven_day, each with used_percentage and resets_at) and
// discards everything else in the payload — model info, cost, context
// window, session id, transcript path, git branch, working directory, etc.
// never reach the returned struct.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/valleedev/iameter-collector/internal/model"
)

const Name = "claude"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return Name }

// rawEnvelope only declares the one field IA METER is allowed to read.
// Any other top-level key in Claude Code's statusLine JSON (model,
// workspace, cost, context_window, session_id, transcript_path, git info,
// ...) is simply never unmarshaled into anything — Go's encoding/json
// ignores unknown fields by default, which is what enforces the whitelist
// at the type level rather than by filtering after the fact.
type rawEnvelope struct {
	RateLimits *rawRateLimits `json:"rate_limits"`
}

type rawRateLimits struct {
	FiveHour *rawWindow `json:"five_hour"`
	SevenDay *rawWindow `json:"seven_day"`
}

type rawWindow struct {
	UsedPercentage json.RawMessage `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}

// Parse implements providers.UsageProvider.
func (p *Provider) Parse(r io.Reader) (*model.RateLimits, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("claude: read input: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("claude: empty input")
	}

	var env rawEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("claude: malformed JSON: %w", err)
	}

	var rl model.RateLimits
	if env.RateLimits != nil {
		rl.FiveHour = buildWindow(env.RateLimits.FiveHour)
		rl.SevenDay = buildWindow(env.RateLimits.SevenDay)
	}
	return &rl, nil
}

// buildWindow validates and converts one rate-limit window. It returns nil
// (window absent) rather than an error whenever the window is missing,
// null, or has an invalid/out-of-range value — per the spec, absent data
// must never be represented by an invented value such as 0.
func buildWindow(w *rawWindow) *model.RateWindow {
	if w == nil {
		return nil
	}
	if isJSONNull(w.UsedPercentage) || isJSONNull(w.ResetsAt) {
		return nil
	}

	var pct float64
	if err := json.Unmarshal(w.UsedPercentage, &pct); err != nil {
		return nil // wrong type, e.g. a string
	}
	if pct < 0 || pct > 100 {
		return nil // out of range
	}

	var resetsAt int64
	if err := json.Unmarshal(w.ResetsAt, &resetsAt); err != nil {
		return nil // wrong type or non-integer, e.g. "soon" or 123.5
	}
	if resetsAt < 0 {
		return nil // not a valid unix timestamp
	}

	return &model.RateWindow{UsedPercentage: pct, ResetsAt: resetsAt}
}

func isJSONNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}
