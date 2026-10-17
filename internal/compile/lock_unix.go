//go:build unix

package compile

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lock serializes compiles of one project across processes. The lock file
// lives in the system temp directory, never in the project, and the kernel
// releases it when a process dies, so there is nothing stale to clean up.
func lock(root string) (unlock func(), err error) {
	sum := sha256.Sum256([]byte(root))
	path := filepath.Join(os.TempDir(), fmt.Sprintf("lfm-%x.lock", sum[:8]))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("compile lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("compile lock: %w", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
