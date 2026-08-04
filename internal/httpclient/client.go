// Package httpclient is the low-level HTTP transport shared by pairing and
// sync: fixed timeout, User-Agent, and JSON helpers. Retry/backoff policy
// lives one layer up (internal/syncer, internal/daemon) because pairing
// and usage-sync have different retry semantics (section 15/16/17).
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/iameter/collector/internal/version"
)

// Timeout bounds a single HTTP request/response round trip.
const Timeout = 15 * time.Second

// Client wraps http.Client with IA METER's base URL and headers.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: Timeout},
	}
}

// Response is a decoded HTTP response: status code, raw body, and the
// subset of headers callers care about (Retry-After).
type Response struct {
	StatusCode int
	Body       []byte
	RetryAfter time.Duration // 0 if the header was absent/unparseable
}

// PostJSON sends body as a JSON POST to path (relative to BaseURL), with
// the given extra headers (e.g. Authorization, Idempotency-Key) plus a
// standard User-Agent and Content-Type. It never retries — callers own
// retry policy.
func (c *Client) PostJSON(ctx context.Context, path string, body any, headers map[string]string) (*Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("httpclient: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("httpclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "IAMeter-Collector/"+version.Version)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("httpclient: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Bound response body size defensively — a malicious or misbehaving
	// backend must not be able to exhaust memory (section 24: "respuestas
	// HTTP maliciosas").
	const maxResponseBytes = 1 << 20
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("httpclient: read response: %w", err)
	}
	if len(respBody) > maxResponseBytes {
		return nil, fmt.Errorf("httpclient: response exceeds maximum size of %d bytes", maxResponseBytes)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}, nil
}

// Ping does a bare GET against BaseURL with a short timeout, for
// connectivity diagnostics (`iameter doctor`). Any HTTP response — even a
// 404 for an undefined route — proves the backend is reachable; only a
// transport-level error means it isn't.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("httpclient: build ping request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("httpclient: unreachable: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := time.ParseDuration(header + "s"); err == nil && secs > 0 {
		return secs
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
