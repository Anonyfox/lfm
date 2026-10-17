//go:build unix

package compile

import (
	"fmt"
	"os"
	"syscall"
)

// lock serializes compiles of one project across processes by flocking the
// project directory itself: no lock file is created anywhere, and the kernel
// releases the lock when a process dies, so nothing can go stale.
func lock(root string) (unlock func(), err error) {
	dir, err := os.Open(root)
	if err != nil {
		return nil, fmt.Errorf("compile lock: %w", err)
	}
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX); err != nil {
		dir.Close()
		return nil, fmt.Errorf("compile lock: %w", err)
	}
	return func() {
		syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)
		dir.Close()
	}, nil
}
