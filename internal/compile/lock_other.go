//go:build !unix

package compile

import "context"

// Without flock, concurrent compiles of one project are not serialized.
func lock(context.Context, string, func()) (func(), error) { return func() {}, nil }
