package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneAgeAndCount(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, category := range []string{"frames", "lighthouse", "libreoffice", "release"} {
		dir := Dir(category)
		if filepath.Dir(dir) != filepath.Join(userCache(t), "lfm") {
			t.Fatal(dir)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 130; i++ {
			path := filepath.Join(dir, fmt.Sprintf("%03d.json", i))
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			date := time.Now().Add(-time.Duration(i) * time.Minute)
			if err := os.Chtimes(path, date, date); err != nil {
				t.Fatal(err)
			}
		}
		stale := filepath.Join(dir, "stale")
		if err := os.Mkdir(stale, 0o700); err != nil {
			t.Fatal(err)
		}
		date := time.Now().Add(-31 * 24 * time.Hour)
		if err := os.Chtimes(stale, date, date); err != nil {
			t.Fatal(err)
		}
		Prune(dir)
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 128 {
			t.Fatalf("%s: %d %v", category, len(entries), err)
		}
		for _, name := range []string{"stale", "129.json", "128.json"} {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Errorf("kept %s", name)
			}
		}
	}
}

func TestDirFallsBackToTemp(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("user cache available without HOME")
	}
	if got, want := Dir("frames"), filepath.Join(os.TempDir(), "lfm", "frames"); got != want {
		t.Fatalf("%s != %s", got, want)
	}
}

// userCache is the platform cache root, which is not XDG_CACHE_HOME on macOS.
func userCache(t *testing.T) string {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
