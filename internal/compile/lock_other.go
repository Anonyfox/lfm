//go:build !unix

package compile

// Without flock, concurrent compiles of one project are not serialized.
func lock(string) (func(), error) { return func() {}, nil }
