//go:build !unix

package release

import "errors"

func lockProfile(string) (func(), error) { return nil, errors.New("profile locking unavailable") }
