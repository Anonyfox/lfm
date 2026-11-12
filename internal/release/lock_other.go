//go:build !unix

package release

import "context"

// AcquireBrowser is a no-op on systems without Unix flock.
func AcquireBrowser(context.Context) (func(), error) { return func() {}, nil }
