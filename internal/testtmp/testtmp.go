// Package testtmp gives a package's tests a temp directory of their own:
// release frames, Chrome profiles and Lighthouse reports land in the system
// temp directory by design, and test projects get a new root every run.
package testtmp

import (
	"fmt"
	"os"
	"testing"
)

// Main runs the tests with TMPDIR inside a fresh directory and removes it
// afterwards. Use it as a package's TestMain.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "lfm-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("TMPDIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
