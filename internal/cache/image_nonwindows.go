//go:build !windows

package cache

import (
	"errors"
	"syscall"
)

func crossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
