package cli

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/testtmp"
)

func TestMain(m *testing.M) {
	// Whole-file hashes include archive stamps. Give every test run the same
	// binary identity instead of the temporary test executable's size/mtime.
	buildid.Writer = func() string { return "lfm-cli-test" }
	testtmp.Main(m, testtmp.Lighthouse)
}
