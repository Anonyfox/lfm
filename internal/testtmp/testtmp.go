// Package testtmp gives a package's tests a temp directory of their own:
// working files and caches stay isolated from the user.
package testtmp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/scratch"
)

// Main runs the tests with color disabled and TMPDIR inside a fresh directory,
// applies optional fixtures, then removes the directory afterwards.
func Main(m *testing.M, setup ...func(string) error) {
	os.Unsetenv("FORCE_COLOR")
	os.Setenv("NO_COLOR", "1")
	scratch.Start()
	dir, err := scratch.Dir("tests")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("TMPDIR", dir)
	os.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	os.Setenv("HOME", dir)
	for _, fixture := range setup {
		if err := fixture(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			scratch.Cleanup()
			os.Exit(1)
		}
	}
	code := m.Run()
	scratch.Cleanup()
	os.Exit(code)
}
