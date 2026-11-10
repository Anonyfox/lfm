package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneAgeSizeAndConfinement(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		size int64
		age  time.Duration
	}{
		{"frames/old/frame.jpg", 1, 49 * time.Hour},
		{"lighthouse/least.json", 160 << 20, 2 * time.Hour},
		{"frames/recent/frame.jpg", 160 << 20, time.Hour},
		{"release/new.json", 1, 0},
	} {
		path := Dir(item.name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = f.Truncate(item.size)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		date := time.Now().Add(-item.age)
		_ = os.Chtimes(path, date, date)
		if strings.HasPrefix(item.name, "frames/") {
			_ = os.Chtimes(filepath.Dir(path), date, date)
		}
	}
	if err := os.Symlink(filepath.Dir(outside), Dir("foreign")); err != nil {
		t.Fatal(err)
	}
	Prune()
	for _, item := range []struct {
		name string
		keep bool
	}{
		{"frames/old", false}, {"lighthouse/least.json", false},
		{"frames/recent/frame.jpg", true}, {"release/new.json", true},
	} {
		_, err := os.Stat(Dir(item.name))
		if (err == nil) != item.keep {
			t.Errorf("%s: keep=%v, err=%v", item.name, item.keep, err)
		}
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "outside" {
		t.Fatal("touched outside cache", err)
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
