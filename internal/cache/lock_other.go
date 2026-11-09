//go:build !unix

package cache

import "errors"

func Lock(string) (func(), error) { return nil, errors.New("profile locking unavailable") }
