//go:build !unix

package scratch

import "os"

// Without an ownership check, leave stale entries alone.
func owned(_ os.FileInfo) bool { return false }
