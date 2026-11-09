//go:build unix

package cache

import (
	"os"
	"syscall"
)

// Lock exclusively locks a cache directory until close or process exit.
func Lock(path string) (func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
