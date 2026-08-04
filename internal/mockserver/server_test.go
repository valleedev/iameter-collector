package mockserver

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/model"
	"github.com/iameter/collector/internal/pairing"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/syncer"
)

type memCreds struct{ values map[string][]byte }

func newMemCreds() *memCreds { return &memCreds{values: map[string][]byte{}} }

func (m *memCreds) Save(key string, value []byte) error { m.values[key] = value; return nil }
func (m *memCreds) Load(key string) ([]byte, error) {
	v, ok := m.values[key]
	if !ok {
		return nil, credentials.ErrNotFound
	}
	return v, nil
}
func (m *memCreds) Delete(key string) error { delete(m.values, key); return nil }
func (m *memCreds) Name() string            { return "mem" }
func (m *memCreds) IsFallback() bool        { return false }

func TestMockServerPairThenSyncEndToEnd(t *testing.T) {
	ms := New()
	code := ms.NewPairingCode()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()

	client := httpclient.New(srv.URL)

	result, err := pairing.Pair(context.Background(), client, code, pairing.DeviceInfo{
		Name: "test", OS: "linux", Arch: "amd64", CollectorVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if result.DeviceID == "" || result.DeviceToken == "" {
		t.Fatalf("result = %+v", result)
	}

	creds := newMemCreds()
	creds.Save("device_token", []byte(result.DeviceToken))

	q, err := queue.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.UsageSnapshot{
		DeviceID:         result.DeviceID,
		Provider:         "claude",
		CollectorVersion: "0.1.0-test",
		CapturedAt:       "2026-08-03T21:40:00Z",
		Platform:         model.Platform{OS: "linux", Arch: "amd64"},
		RateLimits: model.RateLimits{
			FiveHour: &model.RateWindow{UsedPercentage: 50, ResetsAt: 1738425600},
		},
	}
	if _, err := q.Enqueue(snapshot); err != nil {
		t.Fatal(err)
	}

	s := syncer.New(q, client, creds)
	syncResult, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if syncResult.Synced != 1 {
		t.Errorf("Synced = %d, want 1", syncResult.Synced)
	}
	if ms.UsageCount() != 1 {
		t.Errorf("UsageCount() = %d, want 1", ms.UsageCount())
	}

	n, _ := q.Len()
	if n != 0 {
		t.Errorf("Len() = %d, want 0", n)
	}
}

// Regression test: a preset pairing code passed to New(...) (as
// `iameter mock-server --pairing-code X` does) must actually be usable —
// this was broken (only NewPairingCode()-generated codes worked) until it
// was caught by manual end-to-end testing of the CLI.
func TestMockServerPresetPairingCodeUsable(t *testing.T) {
	ms := New("CM-PRESET1")
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	result, err := pairing.Pair(context.Background(), client, "CM-PRESET1", pairing.DeviceInfo{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Pair() with preset code error = %v", err)
	}
	if result.DeviceID == "" {
		t.Error("empty DeviceID")
	}
}

func TestMockServerRejectsReusedPairingCode(t *testing.T) {
	ms := New()
	code := ms.NewPairingCode()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	device := pairing.DeviceInfo{Name: "t", OS: "linux", Arch: "amd64", CollectorVersion: "0.1.0"}
	if _, err := pairing.Pair(context.Background(), client, code, device); err != nil {
		t.Fatalf("first Pair() error = %v", err)
	}
	_, err := pairing.Pair(context.Background(), client, code, device)
	if err == nil {
		t.Fatal("second Pair() with same code: error = nil, want error")
	}
}

func TestMockServerRejectsUnknownCode(t *testing.T) {
	ms := New()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	_, err := pairing.Pair(context.Background(), client, "CM-NOTREAL", pairing.DeviceInfo{OS: "linux", Arch: "amd64"})
	if err == nil {
		t.Fatal("Pair() with unknown code: error = nil, want error")
	}
}

func TestMockServerUsageRejectsUnauthenticated(t *testing.T) {
	ms := New()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	resp, err := client.PostJSON(context.Background(), "/v1/collector/usage", map[string]string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}

func TestMockServerUsageIdempotentReplay(t *testing.T) {
	ms := New()
	code := ms.NewPairingCode()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	result, err := pairing.Pair(context.Background(), client, code, pairing.DeviceInfo{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{
		"Authorization":   "Bearer " + result.DeviceToken,
		"Idempotency-Key": "snap_fixed_id",
	}
	resp1, err := client.PostJSON(context.Background(), "/v1/collector/usage", map[string]string{"x": "1"}, headers)
	if err != nil {
		t.Fatal(err)
	}
	if resp1.StatusCode != 201 {
		t.Fatalf("first request status = %d, want 201", resp1.StatusCode)
	}
	resp2, err := client.PostJSON(context.Background(), "/v1/collector/usage", map[string]string{"x": "1"}, headers)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 409 {
		t.Errorf("replay status = %d, want 409", resp2.StatusCode)
	}
	if ms.UsageCount() != 1 {
		t.Errorf("UsageCount() = %d, want 1 (replay must not double-count)", ms.UsageCount())
	}
}

func TestMockServerHeartbeat(t *testing.T) {
	ms := New()
	code := ms.NewPairingCode()
	srv := httptest.NewServer(ms.Handler())
	defer srv.Close()
	client := httpclient.New(srv.URL)

	result, err := pairing.Pair(context.Background(), client, code, pairing.DeviceInfo{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.PostJSON(context.Background(), "/v1/devices/heartbeat",
		map[string]string{"device_id": result.DeviceID}, map[string]string{"Authorization": "Bearer " + result.DeviceToken})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("StatusCode = %d, want 204", resp.StatusCode)
	}
}
