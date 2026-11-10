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
