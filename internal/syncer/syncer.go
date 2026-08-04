// Package syncer sends queued snapshots to the backend (section 17). It
// makes exactly one pass over the pending queue per SyncOnce call, in
// order, stopping at the first item that can't be confirmed delivered —
// callers (daemon, `iameter sync`) decide whether/when to try again.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/version"
)

const usageEndpoint = "/v1/collector/usage"
const deviceTokenKey = "device_token"

// ErrNotPaired means there is no stored device token — sync cannot proceed
// until `iameter pair` succeeds.
var ErrNotPaired = errors.New("syncer: device is not paired")

// ErrUnauthorized means the backend rejected the device token (401/403).
// Per section 15, callers must stop automatic retries entirely on this —
// it won't resolve itself, the device needs to be re-paired.
var ErrUnauthorized = errors.New("syncer: device token rejected by backend (401/403) — re-pair required")

type Syncer struct {
	Queue  *queue.Queue
	Client *httpclient.Client
	Creds  credentials.Store
}

func New(q *queue.Queue, client *httpclient.Client, creds credentials.Store) *Syncer {
	return &Syncer{Queue: q, Client: client, Creds: creds}
}

// Result summarizes one SyncOnce pass.
type Result struct {
	Synced        int
	Remaining     int
	StoppedReason string        // empty if the queue fully drained
	RetryAfter    time.Duration // set when StoppedReason indicates a rate limit
}

// SyncOnce attempts to deliver every pending item, in FIFO order, stopping
// at the first one that isn't confirmed delivered. It never blocks
// indefinitely — each request uses httpclient's fixed per-request timeout,
// and SyncOnce itself makes no retry loop (the caller decides whether to
// call it again, with what backoff).
func (s *Syncer) SyncOnce(ctx context.Context) (Result, error) {
	tokenBytes, err := s.Creds.Load(deviceTokenKey)
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			return Result{}, ErrNotPaired
		}
		return Result{}, fmt.Errorf("syncer: load device token: %w", err)
	}
	token := string(tokenBytes)

	items, err := s.Queue.Pending()
	if err != nil {
		return Result{}, fmt.Errorf("syncer: read queue: %w", err)
	}

	result := Result{Remaining: len(items)}
	for _, item := range items {
		headers := map[string]string{
			"Authorization":   "Bearer " + token,
			"Idempotency-Key": item.ID,
		}
		resp, err := s.Client.PostJSON(ctx, usageEndpoint, item.Snapshot, headers)
		if err != nil {
			result.StoppedReason = "network_error"
			return result, nil
		}

		switch {
		case resp.StatusCode == http.StatusOK, resp.StatusCode == http.StatusCreated, resp.StatusCode == http.StatusConflict:
			// 200/201: accepted. 409: the backend already has this
			// Idempotency-Key from a prior attempt — treat as delivered,
			// not an error (section 17: idempotency across retries).
			if err := s.Queue.Ack([]string{item.ID}); err != nil {
				return result, fmt.Errorf("syncer: ack: %w", err)
			}
			result.Synced++
			result.Remaining--

		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			result.StoppedReason = "unauthorized"
			return result, ErrUnauthorized

		case resp.StatusCode == http.StatusTooManyRequests:
			_ = s.Queue.IncrementAttempts(item.ID)
			result.StoppedReason = "rate_limited"
			result.RetryAfter = resp.RetryAfter
			return result, nil

		case resp.StatusCode >= 500:
			_ = s.Queue.IncrementAttempts(item.ID)
			result.StoppedReason = fmt.Sprintf("server_error_%d", resp.StatusCode)
			result.RetryAfter = resp.RetryAfter
			return result, nil

		default:
			_ = s.Queue.IncrementAttempts(item.ID)
			result.StoppedReason = fmt.Sprintf("unexpected_status_%d", resp.StatusCode)
			return result, nil
		}
	}

	return result, nil
}

// Heartbeat pings the backend to signal the collector is alive even when
// there's nothing new to sync (section 17: "Incluye un endpoint de
// heartbeat"). It does not touch the queue.
func (s *Syncer) Heartbeat(ctx context.Context, deviceID string) error {
	tokenBytes, err := s.Creds.Load(deviceTokenKey)
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			return ErrNotPaired
		}
		return fmt.Errorf("syncer: load device token: %w", err)
	}

	body := map[string]string{
		"device_id":         deviceID,
		"collector_version": version.Version,
	}
	resp, err := s.Client.PostJSON(ctx, "/v1/devices/heartbeat", body, map[string]string{
		"Authorization": "Bearer " + string(tokenBytes),
	})
	if err != nil {
		return fmt.Errorf("syncer: heartbeat: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	default:
		return fmt.Errorf("syncer: heartbeat: unexpected status %d", resp.StatusCode)
	}
}
