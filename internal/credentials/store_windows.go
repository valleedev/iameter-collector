//go:build windows

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/iameter/collector/internal/fsutil"
)

// dpapiStore encrypts secrets with Windows DPAPI (CryptProtectData, tied
// to the current user account) before writing them to disk. This uses only
// the standard library's syscall package (crypt32.dll/kernel32.dll via
// LazyDLL) — no CGO, no external dependencies (section 8).
type dpapiStore struct {
	dir string
}

func (d dpapiStore) Name() string     { return "windows-dpapi" }
func (d dpapiStore) IsFallback() bool { return false }

func (d dpapiStore) path(key string) (string, error) {
	if key == "" || key != filepath.Base(key) {
		return "", fmt.Errorf("credentials: invalid key %q", key)
	}
	return filepath.Join(d.dir, key+".dpapi"), nil
}

func (d dpapiStore) Save(key string, value []byte) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	encrypted, err := dpapiProtect(value)
	if err != nil {
		return fmt.Errorf("credentials: dpapi protect: %w", err)
	}
	return fsutil.AtomicWriteFile(path, encrypted, 0o600)
}

func (d dpapiStore) Load(key string) ([]byte, error) {
	path, err := d.path(key)
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
	decrypted, err := dpapiUnprotect(data)
	if err != nil {
		return nil, fmt.Errorf("credentials: dpapi unprotect: %w", err)
	}
	return decrypted, nil
}

func (d dpapiStore) Delete(key string) error {
	path, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("credentials: delete: %w", err)
	}
	return nil
}

func platformStore(dir string) (Store, bool) {
	return dpapiStore{dir: filepath.Join(dir, "credentials")}, true
}

// -- DPAPI syscall plumbing --

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	modCrypt32    = syscall.NewLazyDLL("crypt32.dll")
	modKernel32   = syscall.NewLazyDLL("kernel32.dll")
	procProtect   = modCrypt32.NewProc("CryptProtectData")
	procUnprotect = modCrypt32.NewProc("CryptUnprotectData")
	procLocalFree = modKernel32.NewProc("LocalFree")
)

func newBlob(data []byte) dataBlob {
	if len(data) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
}

func blobBytes(b dataBlob) []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func dpapiProtect(plaintext []byte) ([]byte, error) {
	in := newBlob(plaintext)
	var out dataBlob
	ret, _, err := procProtect.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}

func dpapiUnprotect(ciphertext []byte) ([]byte, error) {
	in := newBlob(ciphertext)
	var out dataBlob
	ret, _, err := procUnprotect.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}
