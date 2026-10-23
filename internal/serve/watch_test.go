package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatcherDetectsPublicWellKnownChanges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := newMonitor(root)
	before := m.snapshot()
	dir := filepath.Join(root, "public", ".well-known")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "security.txt")
	for _, content := range []string{"first", "other"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		after := m.snapshot()
		if !changed(before, after) {
			t.Fatal("public .well-known creation or edit was not detected")
		}
		before = after
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !changed(before, m.snapshot()) {
		t.Fatal("public .well-known deletion was not detected")
	}
}

func TestWatcherIncludesResolvedHiddenInputs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".keys")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(path, []byte("key"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newMonitor(root)
	m.include([]string{path})
	before := m.snapshot()
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !changed(before, m.snapshot()) {
		t.Fatal("resolved input edit was not detected")
	}
}
