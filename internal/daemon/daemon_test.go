package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iameter/collector/internal/config"
	"github.com/iameter/collector/internal/credentials"
	"github.com/iameter/collector/internal/fsutil"
	"github.com/iameter/collector/internal/httpclient"
	"github.com/iameter/collector/internal/model"
	"github.com/iameter/collector/internal/queue"
	"github.com/iameter/collector/internal/syncer"
)

func acquireTestLock(t *testing.T, dataDir string) (*fsutil.FileLock, error) {
	t.Helper()
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return fsutil.AcquireFileLock(lockPath(dataDir), time.Second)
}

type memCreds struct{ values map[string][]byte }

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

func pairedDeviceConfig(t *testing.T, configDir string) {
	t.Helper()
	err := config.SaveDeviceConfig(configDir, config.DeviceConfig{
		DeviceID: "dev_test", Paired: true, UserID: "usr_test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunSingleInstanceLock(t *testing.T) {
	dataDir := t.TempDir()
	lock, err := acquireTestLock(t, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	cfg := Config{
		Syncer:      syncer.New(mustQueue(t), httpclient.New("http://unused"), newMemCreds("")),
		ConfigDir:   t.TempDir(),
		DataDir:     dataDir,
		LockTimeout: 100 * time.Millisecond,
	}
	err = Run(context.Background(), cfg)
	if err != ErrAlreadyRunning {
		t.Errorf("Run() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	cfg := Config{
		Syncer:       syncer.New(mustQueue(t), httpclient.New("http://unused"), newMemCreds("")),
		ConfigDir:    t.TempDir(),
		DataDir:      t.TempDir(),
		SyncInterval: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v, want nil on graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRunSyncsAndResetsIntervalOnSuccess(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	pairedDeviceConfig(t, configDir)
	q := mustQueue(t)
	q.Enqueue(model.UsageSnapshot{DeviceID: "dev_test", RateLimits: model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 1, ResetsAt: 1}}})

	var ticks int32
	cfg := Config{
		Syncer:       syncer.New(q, httpclient.New(srv.URL), newMemCreds("tok")),
		ConfigDir:    configDir,
		DataDir:      t.TempDir(),
		SyncInterval: 20 * time.Millisecond,
		OnTick: func(next time.Duration, paused bool) {
			atomic.AddInt32(&ticks, 1)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	Run(ctx, cfg)

	if atomic.LoadInt32(&requests) == 0 {
		t.Error("expected at least one sync request")
	}
	n, _ := q.Len()
	if n != 0 {
		t.Errorf("queue Len() = %d, want 0 (synced)", n)
	}
}

func TestRunBackoffOnRepeatedFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	pairedDeviceConfig(t, configDir)
	q := mustQueue(t)
	q.Enqueue(model.UsageSnapshot{DeviceID: "dev_test", RateLimits: model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 1, ResetsAt: 1}}})

	var intervals []time.Duration
	cfg := Config{
		Syncer:       syncer.New(q, httpclient.New(srv.URL), newMemCreds("tok")),
		ConfigDir:    configDir,
		DataDir:      t.TempDir(),
		SyncInterval: 5 * time.Millisecond,
		MaxBackoff:   200 * time.Millisecond,
		OnTick: func(next time.Duration, paused bool) {
			intervals = append(intervals, next)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	Run(ctx, cfg)

	if len(intervals) < 2 {
		t.Fatalf("not enough ticks observed: %v", intervals)
	}
	if intervals[1] <= intervals[0] {
		t.Errorf("intervals did not grow: %v", intervals)
	}
}

func TestRunPausesOn401AndStopsHammeringBackend(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	pairedDeviceConfig(t, configDir)
	q := mustQueue(t)
	q.Enqueue(model.UsageSnapshot{DeviceID: "dev_test", RateLimits: model.RateLimits{FiveHour: &model.RateWindow{UsedPercentage: 1, ResetsAt: 1}}})

	var pausedSeen int32
	cfg := Config{
		Syncer:                syncer.New(q, httpclient.New(srv.URL), newMemCreds("tok")),
		ConfigDir:             configDir,
		DataDir:               t.TempDir(),
		SyncInterval:          5 * time.Millisecond,
		PausedRecheckInterval: 500 * time.Millisecond, // long: proves we don't retry fast once paused
		OnTick: func(next time.Duration, paused bool) {
			if paused {
				atomic.AddInt32(&pausedSeen, 1)
			}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	Run(ctx, cfg)

	if atomic.LoadInt32(&pausedSeen) == 0 {
		t.Error("expected daemon to report paused state after 401")
	}
	if got := atomic.LoadInt32(&requests); got > 3 {
		t.Errorf("requests = %d, want few (paused should stop hammering the backend)", got)
	}
}

func TestRunHeartbeatFires(t *testing.T) {
	var heartbeats int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/devices/heartbeat" {
			atomic.AddInt32(&heartbeats, 1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	pairedDeviceConfig(t, configDir)

	cfg := Config{
		Syncer:            syncer.New(mustQueue(t), httpclient.New(srv.URL), newMemCreds("tok")),
		ConfigDir:         configDir,
		DataDir:           t.TempDir(),
		SyncInterval:      time.Hour, // don't let sync ticks interfere
		HeartbeatInterval: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Millisecond)
	defer cancel()
	Run(ctx, cfg)

	if atomic.LoadInt32(&heartbeats) == 0 {
		t.Error("expected at least one heartbeat request")
	}
}

func mustQueue(t *testing.T) *queue.Queue {
	t.Helper()
	q, err := queue.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return q
}
