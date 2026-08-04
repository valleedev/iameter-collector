// Package queue implements IA METER's local, persistent, offline-first
// snapshot queue (sections 4 and 14). Every mutating and reading operation
// holds a cross-process file lock (internal/fsutil) and rewrites the
// backing file atomically, so:
//   - concurrent `iameter statusline` invocations never corrupt the file;
//   - a process crash mid-write leaves either the old or the new complete
//     file, never a half-written one (atomic rename);
//   - "compaction" isn't a separate step — every write already rewrites
//     only the live items, so the file never accumulates dead entries.
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/valleedev/iameter-collector/internal/fsutil"
	"github.com/valleedev/iameter-collector/internal/idgen"
	"github.com/valleedev/iameter-collector/internal/model"
)

// MaxItems bounds how many pending snapshots are kept while offline. At
// one snapshot roughly every few minutes of active use, 500 covers several
// days of full disconnection. When exceeded, the oldest items are dropped
// first — the spec requires preserving the most recent snapshot, not the
// oldest (section 14: "conservación del snapshot más reciente").
const MaxItems = 500

// MinReheartbeat is the minimum time between two queued snapshots that
// have identical rate limits — below this, an unchanged snapshot is not
// re-queued (section 14: "No almacenes snapshots idénticos continuamente").
// Above it, an unchanged snapshot is still queued once as a heartbeat so
// the backend can tell the collector is alive.
const MinReheartbeat = 5 * time.Minute

const (
	fileName = "queue.json"
	lockName = "queue.lock"
)

// Item wraps a snapshot with queue bookkeeping. ID is generated once and
// reused as the sync Idempotency-Key across every retry (section 17).
type Item struct {
	ID         string              `json:"id"`
	Snapshot   model.UsageSnapshot `json:"snapshot"`
	EnqueuedAt string              `json:"enqueued_at"`
	Attempts   int                 `json:"attempts"`
}

type document struct {
	Items []Item `json:"items"`
}

// Queue is a handle to the on-disk queue in dataDir. It holds no
// in-memory state between calls — every method reads-modifies-writes the
// backing file under lock, so multiple *Queue values (e.g. in different
// processes) are always safe to use concurrently.
type Queue struct {
	path     string
	lockPath string
}

// Open returns a queue backed by dataDir/queue.json, creating dataDir if
// needed. It does not read the file yet — corruption recovery happens
// lazily on first access, under lock.
func Open(dataDir string) (*Queue, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("queue: create data dir: %w", err)
	}
	return &Queue{
		path:     filepath.Join(dataDir, fileName),
		lockPath: filepath.Join(dataDir, lockName),
	}, nil
}

const lockTimeout = 5 * time.Second

// withLock runs fn while holding the cross-process queue lock.
func (q *Queue) withLock(fn func() error) error {
	lock, err := fsutil.AcquireFileLock(q.lockPath, lockTimeout)
	if err != nil {
		return fmt.Errorf("queue: acquire lock: %w", err)
	}
	defer lock.Release()
	return fn()
}

// load reads and parses the queue file. A missing file is an empty queue.
// A corrupt file is quarantined (renamed, not deleted, so it can be
// inspected) and treated as an empty queue rather than propagating the
// error — a corrupted local queue must never block statusline capture or
// crash the daemon (section 14: "recuperarse de archivos corruptos").
func (q *Queue) load() (document, error) {
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return document{}, nil
		}
		return document{}, fmt.Errorf("queue: read: %w", err)
	}
	if len(data) == 0 {
		return document{}, nil
	}

	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		quarantined := q.path + fmt.Sprintf(".corrupt-%d", time.Now().Unix())
		_ = os.Rename(q.path, quarantined)
		return document{}, nil
	}
	return doc, nil
}

func (q *Queue) save(doc document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("queue: marshal: %w", err)
	}
	return fsutil.AtomicWriteFile(q.path, data, 0o600)
}

// Enqueue appends a snapshot unless it's an exact repeat of the most
// recent one within MinReheartbeat. It returns whether the snapshot was
// actually added.
func (q *Queue) Enqueue(snapshot model.UsageSnapshot) (added bool, err error) {
	id, err := idgen.New("snap")
	if err != nil {
		return false, err
	}

	err = q.withLock(func() error {
		doc, err := q.load()
		if err != nil {
			return err
		}

		if len(doc.Items) > 0 {
			last := doc.Items[len(doc.Items)-1]
			if sameRateLimits(last.Snapshot.RateLimits, snapshot.RateLimits) {
				lastTime, errT := time.Parse(time.RFC3339, last.EnqueuedAt)
				if errT == nil && time.Since(lastTime) < MinReheartbeat {
					added = false
					return nil // duplicate within the heartbeat window; skip
				}
			}
		}

		doc.Items = append(doc.Items, Item{
			ID:         id,
			Snapshot:   snapshot,
			EnqueuedAt: time.Now().UTC().Format(time.RFC3339),
		})
		doc.Items = trimToMax(doc.Items, MaxItems)
		added = true
		return q.save(doc)
	})
	return added, err
}

// trimToMax keeps at most max items, dropping from the front (oldest)
// first so the most recent snapshot is always retained.
func trimToMax(items []Item, max int) []Item {
	if len(items) <= max {
		return items
	}
	return items[len(items)-max:]
}

func sameRateLimits(a, b model.RateLimits) bool {
	return sameWindow(a.FiveHour, b.FiveHour) && sameWindow(a.SevenDay, b.SevenDay)
}

func sameWindow(a, b *model.RateWindow) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.UsedPercentage == b.UsedPercentage && a.ResetsAt == b.ResetsAt
}

// Pending returns all queued items in FIFO order (oldest first).
func (q *Queue) Pending() ([]Item, error) {
	var items []Item
	err := q.withLock(func() error {
		doc, err := q.load()
		if err != nil {
			return err
		}
		items = doc.Items
		return nil
	})
	return items, err
}

// Peek returns up to n of the oldest pending items, for batched sync
// (Phase 5) without removing them.
func (q *Queue) Peek(n int) ([]Item, error) {
	items, err := q.Pending()
	if err != nil {
		return nil, err
	}
	if n >= 0 && len(items) > n {
		items = items[:n]
	}
	return items, nil
}

// Last returns the most recently enqueued item, or nil if the queue is
// empty.
func (q *Queue) Last() (*Item, error) {
	items, err := q.Pending()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	last := items[len(items)-1]
	return &last, nil
}

// Len returns the number of pending items.
func (q *Queue) Len() (int, error) {
	items, err := q.Pending()
	return len(items), err
}

// Ack removes the given item IDs from the queue (successfully synced).
// Unknown IDs are ignored.
func (q *Queue) Ack(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	toRemove := make(map[string]bool, len(ids))
	for _, id := range ids {
		toRemove[id] = true
	}
	return q.withLock(func() error {
		doc, err := q.load()
		if err != nil {
			return err
		}
		kept := doc.Items[:0]
		for _, item := range doc.Items {
			if !toRemove[item.ID] {
				kept = append(kept, item)
			}
		}
		doc.Items = kept
		return q.save(doc)
	})
}

// IncrementAttempts records a failed sync attempt for the given item so
// the syncer (Phase 5) can apply backoff without losing the item's
// position or Idempotency-Key.
func (q *Queue) IncrementAttempts(id string) error {
	return q.withLock(func() error {
		doc, err := q.load()
		if err != nil {
			return err
		}
		found := false
		for i := range doc.Items {
			if doc.Items[i].ID == id {
				doc.Items[i].Attempts++
				found = true
				break
			}
		}
		if !found {
			return nil // already acked/removed by a concurrent sync; not an error
		}
		return q.save(doc)
	})
}
