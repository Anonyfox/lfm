package main

import (
	"os"
	"testing"

	"github.com/Anonyfox/lfm/internal/scratch"
)

func TestErrorExitCleansScratch(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	path, err := scratch.Dir("office")
	if err != nil {
		t.Fatal(err)
	}
	file, err := scratch.File(os.Getenv("XDG_CACHE_HOME"), ".report-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	previous := os.Args
	os.Args = []string{"lfm", "not-a-command"}
	t.Cleanup(func() { os.Args = previous })
	if code := run(); code == 0 {
		t.Fatal("expected error exit")
	}
	for _, name := range []string{path, file.Name()} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("scratch remains: %s: %v", name, err)
		}
	}
}
