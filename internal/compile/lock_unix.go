//go:build unix

package compile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lock serializes compiles of one project across processes by flocking the
// project directory itself: no lock file is created anywhere, and the kernel
// releases the lock when a process dies, so nothing can go stale. While another
// compile holds it, waiting is announced once and stops when ctx is cancelled.
func lock(ctx context.Context, root string, waiting func()) (unlock func(), err error) {
	dir, err := os.Open(root)
	if err != nil {
		return nil, fmt.Errorf("compile lock: %w", err)
	}
	for announced := false; ; {
		err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			dir.Close()
			return nil, fmt.Errorf("compile lock: %w", err)
		}
		if !announced && waiting != nil {
			waiting()
		}
		announced = true
		select {
		case <-ctx.Done():
			dir.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return func() {
		syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)
		dir.Close()
	}, nil
}
