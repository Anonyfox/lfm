//go:build windows

package cache

import (
	"errors"
	"syscall"
)

// Win32 ERROR_NOT_SAME_DEVICE; syscall.EXDEV is a synthetic errno on Windows.
const errorNotSameDevice syscall.Errno = 17

func crossDevice(err error) bool { return errors.Is(err, errorNotSameDevice) }
