// Package fsutil holds small filesystem helpers shared across config,
// settings, and queue: atomic writes and cross-process file locking. Both
// need the same "write temp file in target dir, fsync, rename" and
// "exclusive-create lock file with staleness check" logic, so it lives here
// once instead of being copy-pasted per package.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a temp file in the same directory as path,
// fsyncs it, then renames it into place. Rename is atomic on POSIX and on
// Windows (os.Rename uses MoveFileEx with MOVEFILE_REPLACE_EXISTING since
// Go 1.5). The temp file is created in the target directory so the rename
// is same-filesystem.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("atomic write: create dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("atomic write: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write: close: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("atomic write: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("atomic write: rename: %w", err)
	}
	return nil
}
