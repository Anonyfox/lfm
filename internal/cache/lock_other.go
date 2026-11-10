//go:build !unix

package cache

import (
	"errors"
	"os"
)

func shared(os.FileInfo) bool { return false }

func differentDevices(os.FileInfo, os.FileInfo) bool { return false }

func Lock(string) (func(), error) { return nil, errors.New("profile locking unavailable") }
