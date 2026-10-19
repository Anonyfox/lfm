package buildid

import (
	"strings"
	"testing"
)

func TestWriterIsStableAndNamesThisBinary(t *testing.T) {
	a, b := Writer(), Writer()
	if a == "" || a != b {
		t.Fatalf("%q then %q", a, b)
	}
	// go test builds carry no VCS revision, so the executable names them.
	if !strings.HasPrefix(a, "local+") && !strings.Contains(a, "+") {
		t.Fatalf("unexpected %q", a)
	}
}
