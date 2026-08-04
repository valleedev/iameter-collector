package syncer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/model"
	"github.com/iameter/collector/internal/queue"
)

// memCreds is a minimal in-memory credentials.Store for tests, avoiding
// any dependency on OS-native stores or the filesystem.
type memCreds struct {
	values map[string][]byte
}

func newMemCreds(token string) *memCreds {
	m := &memCreds{values: map[string][]byte{}}
	if token != "" {
		m.values["device_token"] = []byte(token)
	}
	return m
}

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

func snap(pct float64) model.UsageSnapshot {
	return model.UsageSnapshot{
		DeviceID:         "dev_test",
		Provider:         "claude",
		CollectorVersion: "0.1.0-test",
		CapturedAt:       time.Now().UTC().Format(time.RFC3339),
		Platform:         model.Platform{OS: "linux", Arch: "amd64"},
		RateLimits: model.RateLimits{
			FiveHour: &model.RateWindow{UsedPercentage: pct, ResetsAt: 1738425600},
		},
	}
}

func newTestQueue(t *testing.T, snapshots ...model.UsageSnapshot) *queue.Queue {
	t.Helper()
	q, err := queue.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snapshots {
		if _, err := q.Enqueue(s); err != nil {
			t.Fatal(err)
		}
	}
	return q
}

func TestSyncOnceSuccess200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	q := newTestQueue(t, snap(10), snap(20))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.Synced != 2 || res.Remaining != 0 {
		t.Errorf("res = %+v", res)
	}
	n, _ := q.Len()
	if n != 0 {
		t.Errorf("Len() = %d, want 0 (all acked)", n)
	}
}

func TestSyncOnceSuccess201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil || res.Synced != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestSyncOnceTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	client := httpclient.New(srv.URL)
	client.HTTP.Timeout = 50 * time.Millisecond
	q := newTestQueue(t, snap(10))
	s := New(q, client, newMemCreds("tok"))

	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v, want nil (timeout stops batch, not fatal)", err)
	}
	if res.StoppedReason != "network_error" {
		t.Errorf("StoppedReason = %q, want network_error", res.StoppedReason)
	}
	n, _ := q.Len()
	if n != 1 {
		t.Errorf("Len() = %d, want 1 (item stays queued on timeout)", n)
	}
}

func TestSyncOnceNetworkError(t *testing.T) {
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New("http://127.0.0.1:1"), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.StoppedReason != "network_error" {
		t.Errorf("StoppedReason = %q", res.StoppedReason)
	}
}

func TestSyncOnce401StopsAndKeepsItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))

	_, err := s.SyncOnce(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
	n, _ := q.Len()
	if n != 1 {
		t.Errorf("Len() = %d, want 1 (401 must not drop the item)", n)
	}
}

func TestSyncOnce403Stops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	_, err := s.SyncOnce(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestSyncOnce409TreatedAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.Synced != 1 {
		t.Errorf("Synced = %d, want 1 (409 = already delivered, idempotent)", res.Synced)
	}
	n, _ := q.Len()
	if n != 0 {
		t.Errorf("Len() = %d, want 0", n)
	}
}

func TestSyncOnce429RespectsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.StoppedReason != "rate_limited" {
		t.Errorf("StoppedReason = %q", res.StoppedReason)
	}
	if res.RetryAfter < 44*time.Second {
		t.Errorf("RetryAfter = %v, want ~45s", res.RetryAfter)
	}
	items, _ := q.Pending()
	if len(items) != 1 || items[0].Attempts != 1 {
		t.Errorf("items = %+v, want 1 item with Attempts=1", items)
	}
}

func TestSyncOnce500StopsAndIncrementsAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.StoppedReason != "server_error_500" {
		t.Errorf("StoppedReason = %q", res.StoppedReason)
	}
	items, _ := q.Pending()
	if items[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", items[0].Attempts)
	}
}

func TestSyncOnceInvalidResponseStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.StoppedReason == "" {
		t.Error("expected a StoppedReason for an unexpected status code")
	}
}

func TestSyncOnceNotPaired(t *testing.T) {
	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New("http://unused"), newMemCreds(""))
	_, err := s.SyncOnce(context.Background())
	if !errors.Is(err, ErrNotPaired) {
		t.Errorf("error = %v, want ErrNotPaired", err)
	}
}

func TestSyncOncePreservesOrder(t *testing.T) {
	var gotOrder []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrder = append(gotOrder, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q := newTestQueue(t, snap(10), snap(20), snap(30))
	items, _ := q.Pending()
	wantOrder := []string{items[0].ID, items[1].ID, items[2].ID}

	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))
	if _, err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(gotOrder) != 3 || gotOrder[0] != wantOrder[0] || gotOrder[1] != wantOrder[1] || gotOrder[2] != wantOrder[2] {
		t.Errorf("gotOrder = %v, want %v", gotOrder, wantOrder)
	}
}

func TestSyncOnceIdempotencyKeyStableAcrossRetries(t *testing.T) {
	var attempt int32
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if atomic.AddInt32(&attempt, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError) // first attempt fails
			return
		}
		w.WriteHeader(http.StatusOK) // second attempt (simulated retry) succeeds
	}))
	defer srv.Close()

	q := newTestQueue(t, snap(10))
	s := New(q, httpclient.New(srv.URL), newMemCreds("tok"))

	if _, err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != keys[1] {
		t.Errorf("Idempotency-Key changed across retries: %v", keys)
	}
}

func TestSyncOnceEmptyQueue(t *testing.T) {
	q := newTestQueue(t)
	s := New(q, httpclient.New("http://unused"), newMemCreds("tok"))
	res, err := s.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce() error = %v", err)
	}
	if res.Synced != 0 || res.Remaining != 0 {
		t.Errorf("res = %+v", res)
	}
}
