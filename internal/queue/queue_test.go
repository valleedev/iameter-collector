package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iameter/collector/internal/model"
)

func snap(fiveHourPct float64) model.UsageSnapshot {
	return model.UsageSnapshot{
		DeviceID:         "dev_test",
		Provider:         "claude",
		CollectorVersion: "0.1.0-test",
		CapturedAt:       time.Now().UTC().Format(time.RFC3339),
		Platform:         model.Platform{OS: "linux", Arch: "amd64"},
		RateLimits: model.RateLimits{
			FiveHour: &model.RateWindow{UsedPercentage: fiveHourPct, ResetsAt: 1738425600},
		},
	}
}

func TestEnqueueAndPendingOrder(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, pct := range []float64{10, 20, 30} {
		added, err := q.Enqueue(snap(pct))
		if err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		if !added {
			t.Fatalf("Enqueue(%v) added = false, want true", pct)
		}
	}
	items, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	for i, want := range []float64{10, 20, 30} {
		if *items[i].Snapshot.RateLimits.FiveHour != (model.RateWindow{UsedPercentage: want, ResetsAt: 1738425600}) {
			t.Errorf("items[%d] = %+v, want pct %v", i, items[i], want)
		}
	}
}

func TestEnqueueDedupIdenticalWithinHeartbeat(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	added1, _ := q.Enqueue(snap(50))
	added2, _ := q.Enqueue(snap(50)) // identical, immediately after
	if !added1 || added2 {
		t.Errorf("added1=%v added2=%v, want true,false (dedup identical snapshot)", added1, added2)
	}
	n, _ := q.Len()
	if n != 1 {
		t.Errorf("Len() = %d, want 1", n)
	}
}

func TestEnqueueChangedPercentageAlwaysAdded(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(50))
	added, _ := q.Enqueue(snap(51))
	if !added {
		t.Error("changed percentage should always be enqueued")
	}
	n, _ := q.Len()
	if n != 2 {
		t.Errorf("Len() = %d, want 2", n)
	}
}

func TestEnqueueHeartbeatAfterInterval(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(50))

	// Manually backdate the last item's timestamp past MinReheartbeat to
	// simulate time passing, then enqueue an identical snapshot again.
	doc, err := q.load()
	if err != nil {
		t.Fatal(err)
	}
	doc.Items[0].EnqueuedAt = time.Now().Add(-MinReheartbeat - time.Minute).UTC().Format(time.RFC3339)
	if err := q.save(doc); err != nil {
		t.Fatal(err)
	}

	added, err := q.Enqueue(snap(50))
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("identical snapshot after heartbeat interval should be enqueued")
	}
}

func TestAckRemovesItems(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(10))
	q.Enqueue(snap(20))
	items, _ := q.Pending()
	if len(items) != 2 {
		t.Fatalf("setup: len = %d", len(items))
	}

	if err := q.Ack([]string{items[0].ID}); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	remaining, _ := q.Pending()
	if len(remaining) != 1 || remaining[0].ID != items[1].ID {
		t.Errorf("remaining = %+v, want only items[1]", remaining)
	}
}

func TestAckUnknownIDIsNoop(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(10))
	if err := q.Ack([]string{"snap_doesnotexist"}); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	n, _ := q.Len()
	if n != 1 {
		t.Errorf("Len() = %d, want 1 (unknown ack should not remove anything)", n)
	}
}

func TestMaxItemsTrimsOldest(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxItems+10; i++ {
		if _, err := q.Enqueue(snap(float64(i % 100))); err != nil {
			t.Fatal(err)
		}
	}
	n, err := q.Len()
	if err != nil {
		t.Fatal(err)
	}
	if n != MaxItems {
		t.Errorf("Len() = %d, want %d (oldest should be trimmed)", n, MaxItems)
	}
	last, err := q.Last()
	if err != nil {
		t.Fatal(err)
	}
	// The most recently enqueued item must always survive trimming.
	if last.Snapshot.RateLimits.FiveHour.UsedPercentage != float64((MaxItems+9)%100) {
		t.Errorf("Last() pct = %v, want most recent preserved", last.Snapshot.RateLimits.FiveHour.UsedPercentage)
	}
}

func TestCorruptFileRecovery(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() on corrupt file: error = %v, want nil (recovers to empty queue)", err)
	}
	if len(items) != 0 {
		t.Errorf("Pending() on corrupt file = %v, want empty", items)
	}

	// The corrupt file must be quarantined, not silently deleted.
	entries, _ := os.ReadDir(dir)
	foundQuarantine := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), fileName+".corrupt-") {
			foundQuarantine = true
		}
	}
	if !foundQuarantine {
		t.Errorf("expected a quarantined corrupt file in %v, found entries: %v", dir, entries)
	}

	// Queue must still be usable after recovery.
	added, err := q.Enqueue(snap(42))
	if err != nil || !added {
		t.Fatalf("Enqueue() after corruption recovery: added=%v err=%v", added, err)
	}
}

func TestEmptyFileTreatedAsEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(items) != 0 {
		t.Errorf("Pending() = %v, want empty", items)
	}
}

func TestConcurrentEnqueue(t *testing.T) {
	dir := t.TempDir()
	const n = 30
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q, err := Open(dir)
			if err != nil {
				errs <- err
				return
			}
			// Vary the percentage per goroutine so dedup doesn't collapse
			// them — this test is about concurrency safety, not dedup.
			if _, err := q.Enqueue(snap(float64(i))); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Enqueue() error = %v", err)
	}

	q, _ := Open(dir)
	items, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != n {
		t.Errorf("len(items) = %d, want %d (no lost writes under concurrency)", len(items), n)
	}

	// The file itself must always be valid JSON — no torn writes.
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Errorf("queue.json is not valid JSON after concurrent writes: %v", err)
	}
}

func TestLastOnEmptyQueue(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	last, err := q.Last()
	if err != nil {
		t.Fatalf("Last() error = %v", err)
	}
	if last != nil {
		t.Errorf("Last() = %+v, want nil on empty queue", last)
	}
}

func TestPeekDoesNotRemove(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(1))
	q.Enqueue(snap(2))
	q.Enqueue(snap(3))

	peeked, err := q.Peek(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(peeked) != 2 {
		t.Fatalf("len(peeked) = %d, want 2", len(peeked))
	}
	n, _ := q.Len()
	if n != 3 {
		t.Errorf("Len() after Peek = %d, want 3 (Peek must not remove)", n)
	}
}

func TestIncrementAttempts(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q.Enqueue(snap(1))
	items, _ := q.Pending()
	id := items[0].ID

	if err := q.IncrementAttempts(id); err != nil {
		t.Fatalf("IncrementAttempts() error = %v", err)
	}
	if err := q.IncrementAttempts(id); err != nil {
		t.Fatal(err)
	}

	items, _ = q.Pending()
	if items[0].Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", items[0].Attempts)
	}
	if items[0].ID != id {
		t.Error("ID changed across attempts — Idempotency-Key must stay stable")
	}
}

func TestOfflineOperationNoNetwork(t *testing.T) {
	// This test's only assertion is implicit: Open/Enqueue/Pending/Ack use
	// no package that could reach the network (no net/http import in
	// queue.go), so the queue works identically with or without
	// connectivity. Exercise the full lifecycle to be sure nothing blocks.
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue(snap(1)); err != nil {
		t.Fatal(err)
	}
	items, err := q.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Ack([]string{items[0].ID}); err != nil {
		t.Fatal(err)
	}
}
