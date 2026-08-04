// Package model holds the data shapes shared across providers, the queue,
// and the sync client. Keeping them here (instead of duplicated per-package)
// is what lets internal/queue and internal/syncer stay provider-agnostic.
package model

// RateWindow is a single rate-limit window (five_hour or seven_day).
//
// UsedPercentage is a pointer-free value type because the window itself is
// optional (represented by the containing pointer in RateLimits); once a
// window is present, both fields are always known. 0 is a valid percentage,
// so absence is modeled by the window being nil, never by a sentinel value.
type RateWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// RateLimits mirrors the whitelisted subset of Claude Code's statusLine
// rate_limits object. Each window may be independently absent.
type RateLimits struct {
	FiveHour *RateWindow `json:"five_hour,omitempty"`
	SevenDay *RateWindow `json:"seven_day,omitempty"`
}

// Empty reports whether neither window is present.
func (r RateLimits) Empty() bool {
	return r.FiveHour == nil && r.SevenDay == nil
}

// Platform identifies the collector's host OS/architecture.
type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// UsageSnapshot is the normalized, whitelisted payload IA METER stores in
// the local queue and sends to the backend. Every field here is either
// generated locally or copied from the provider's whitelisted rate_limits
// output — never the raw provider JSON.
type UsageSnapshot struct {
	DeviceID         string     `json:"device_id"`
	Provider         string     `json:"provider"`
	CollectorVersion string     `json:"collector_version"`
	CapturedAt       string     `json:"captured_at"` // RFC3339 UTC
	Platform         Platform   `json:"platform"`
	RateLimits       RateLimits `json:"rate_limits"`
}
