// Package pairing implements the device-pairing handshake (section 16):
// POST /v1/devices/pair with a one-time pairing code, exchanged for a
// device token. The pairing code is never persisted after use.
package pairing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/iameter/collector/internal/httpclient"
)

const pairEndpoint = "/v1/devices/pair"

// DeviceInfo is the non-secret device metadata sent when pairing.
type DeviceInfo struct {
	Name             string `json:"name"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	CollectorVersion string `json:"collector_version"`
}

type pairRequest struct {
	PairingCode string     `json:"pairing_code"`
	Device      DeviceInfo `json:"device"`
}

// Result is what a successful pairing yields.
type Result struct {
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
	UserID      string `json:"user_id"`
}

// Sentinel errors so callers (CLI) can print a precise, non-technical
// message for each documented failure mode (section 16).
var (
	ErrInvalidFormat     = errors.New("pairing: invalid pairing code format")
	ErrExpiredOrNotFound = errors.New("pairing: pairing code expired or not found")
	ErrAlreadyUsed       = errors.New("pairing: pairing code already used")
	ErrAlreadyPaired     = errors.New("pairing: device already paired")
	ErrServer            = errors.New("pairing: backend error")
	ErrInvalidResponse   = errors.New("pairing: backend returned an invalid response")
)

// Pair exchanges a pairing code for device credentials.
func Pair(ctx context.Context, client *httpclient.Client, code string, device DeviceInfo) (*Result, error) {
	resp, err := client.PostJSON(ctx, pairEndpoint, pairRequest{PairingCode: code, Device: device}, nil)
	if err != nil {
		return nil, fmt.Errorf("pairing: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var result Result
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
		}
		if result.DeviceID == "" || result.DeviceToken == "" {
			return nil, fmt.Errorf("%w: missing device_id/device_token", ErrInvalidResponse)
		}
		return &result, nil
	case http.StatusBadRequest:
		return nil, ErrInvalidFormat
	case http.StatusNotFound, http.StatusGone:
		return nil, ErrExpiredOrNotFound
	case http.StatusConflict:
		return nil, ErrAlreadyUsed
	case http.StatusForbidden:
		return nil, ErrAlreadyPaired
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrServer, resp.StatusCode)
	}
}
