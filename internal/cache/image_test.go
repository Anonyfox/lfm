package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageLinksSurvivePruningAndReleaseTheirBudget(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, path := range []string{a, b} {
		if err := LinkImage(path, []byte("image")); err != nil {
			t.Fatal(err)
		}
	}
	ai, _ := os.Stat(a)
	bi, _ := os.Stat(b)
	if !os.SameFile(ai, bi) {
		t.Fatal("identical outputs do not share an inode")
	}
	if !shared(ai) {
		t.Skip("link count unavailable")
	}
	entries, err := os.ReadDir(Dir("img"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache: %v %v", entries, err)
	}
	path := filepath.Join(Dir("img"), entries[0].Name())
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	Prune()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("pruned active image", err)
	}
	for _, output := range []string{a, b} {
		if err := os.Remove(output); err != nil {
			t.Fatal(err)
		}
	}
	Prune()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unused expired image retained", err)
	}
}

func TestImageCopyAcrossFilesystems(t *testing.T) {
	base, err := os.MkdirTemp("/dev/shm", "lfm-image-test-")
	if err != nil {
		t.Skip("second filesystem unavailable:", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	target := filepath.Join(t.TempDir(), "image")
	a, _ := os.Stat(base)
	b, _ := os.Stat(filepath.Dir(target))
	if !differentDevices(a, b) {
		t.Skip("second filesystem unavailable")
	}
	t.Setenv("XDG_CACHE_HOME", base)
	contents := []byte("image")
	if err := LinkImage(target, contents); err != nil {
		t.Fatal(err)
	}
	if !ImageReusable(target, contents) {
		t.Fatal("cross-device output would be republished")
	}
	path := imagePath(contents)
	cached, _ := os.Stat(path)
	output, _ := os.Stat(target)
	if os.SameFile(cached, output) {
		t.Fatal("expected a cross-device copy")
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	Prune()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expired unlinked image retained", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != string(contents) {
		t.Fatal("output lost after pruning", err)
	}
}
