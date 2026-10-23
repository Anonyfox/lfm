// Package testtmp gives a package's tests a temp directory of their own:
// release frames, Chrome profiles and Lighthouse reports land in the system
// temp directory by design, and test projects get a new root every run.
package testtmp

import (
	"fmt"
	"os"
	"testing"
)

// Main runs the tests with color disabled and TMPDIR inside a fresh directory,
// then removes it afterwards. Use it as a package's TestMain.
func Main(m *testing.M) {
	os.Unsetenv("FORCE_COLOR")
	os.Setenv("NO_COLOR", "1")
	// /tmp before the system temp directory: esbuild reads every directory
	// above a project on each build, and a macOS user temp directory can
	// hold a hundred thousand entries that slow every compile tenfold.
	dir, err := os.MkdirTemp("/tmp", "lfm-tests-")
	if err != nil {
		dir, err = os.MkdirTemp("", "lfm-tests-")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("TMPDIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
