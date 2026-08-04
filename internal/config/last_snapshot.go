package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iameter/collector/internal/fsutil"
	"github.com/iameter/collector/internal/model"
)

const lastSnapshotFile = "last_snapshot.json"

// SaveLastSnapshot caches the most recently captured usage snapshot,
// independent of the sync queue's pending/acked lifecycle — `iameter
// status` (section 23) must keep showing the last known consumption even
// after everything has been successfully synced and the queue is empty.
func SaveLastSnapshot(configDir string, snapshot model.UsageSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal last snapshot: %w", err)
	}
	return fsutil.AtomicWriteFile(filepath.Join(configDir, lastSnapshotFile), data, 0o600)
}

// LoadLastSnapshot returns the cached snapshot, or (zero value, false) if
// none has been captured yet.
func LoadLastSnapshot(configDir string) (model.UsageSnapshot, bool, error) {
	data, err := os.ReadFile(filepath.Join(configDir, lastSnapshotFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.UsageSnapshot{}, false, nil
		}
		return model.UsageSnapshot{}, false, fmt.Errorf("config: read last snapshot: %w", err)
	}
	var snapshot model.UsageSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return model.UsageSnapshot{}, false, fmt.Errorf("config: parse last snapshot: %w", err)
	}
	return snapshot, true, nil
}
