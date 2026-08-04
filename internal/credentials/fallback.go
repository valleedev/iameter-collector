package credentials

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/valleedev/iameter-collector/internal/fsutil"
)

// fileStore is the universal fallback: one file per key, restrictive
// permissions (0600), no OS keychain integration. Used when no OS-native
// secret store is available.
type fileStore struct {
	dir string
}

func newFileStore(dir string) *fileStore {
	return &fileStore{dir: filepath.Join(dir, "credentials")}
}

func (f *fileStore) Name() string     { return "file-fallback" }
func (f *fileStore) IsFallback() bool { return true }

func (f *fileStore) keyPath(key string) (string, error) {
	if key == "" || key != filepath.Base(key) {
		return "", fmt.Errorf("credentials: invalid key %q", key)
	}
	return filepath.Join(f.dir, key+".cred"), nil
}

func (f *fileStore) Save(key string, value []byte) error {
	path, err := f.keyPath(key)
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(path, value, 0o600)
}

func (f *fileStore) Load(key string) ([]byte, error) {
	path, err := f.keyPath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("credentials: read: %w", err)
	}
	return data, nil
}

func (f *fileStore) Delete(key string) error {
	path, err := f.keyPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("credentials: delete: %w", err)
	}
	return nil
}
