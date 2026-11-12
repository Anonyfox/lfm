package compile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/testproject"
)

func TestFastReleaseDoesNotLaunchBrowser(t *testing.T) {
	root := fixture(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("LFM_TEST_TOOL_LOG", log)
	for _, name := range []string{"LFM_CHROME", "LFM_LIGHTHOUSE"} {
		tool := filepath.Join(t.TempDir(), "tool")
		if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LFM_TEST_TOOL_LOG\"\necho fixture-version\n"), 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, tool)
	}
	var before string
	for i := 0; i < 2; i++ {
		result, err := Compile(context.Background(), root, Options{Release: true, Publish: true})
		if err != nil || !result.Published || result.Validated {
			t.Fatalf("%v %+v", err, result)
		}
		count := 0
		for _, d := range result.Validation.Diagnostics {
			if d.Rule == "release.validate" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("validation warnings: %d", count)
		}
		raw, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Fields(string(raw)) {
			if line != "--version" {
				t.Fatalf("render launched: %q", raw)
			}
		}
		if i == 1 && before != string(raw) {
			t.Fatal("warm release probed tools again")
		}
		before = string(raw)
	}
}

func TestFastReleasePendingPreservesOutput(t *testing.T) {
	root := testproject.Printable(t)
	if _, err := Compile(context.Background(), root, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	before := read(t, root, "output/index.html")
	result, err := Compile(context.Background(), root, Options{Release: true, Publish: true})
	if err == nil || result.Published {
		t.Fatalf("pending release published: %v", err)
	}
	found := false
	for _, d := range result.Validation.Diagnostics {
		if d.Rule == "release.pending" && d.Severity == "error" && strings.Contains(d.Message, ".pdf") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing pending file error: %+v", result.Validation)
	}
	if before != read(t, root, "output/index.html") {
		t.Fatal("failed release replaced output")
	}
}
