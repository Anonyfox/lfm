package release

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/cache"
)

func TestBrowserProfileReuseFallbackAndFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell and directory locks")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	first, release, err := browserProfile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	fallback, cleanup, err := browserProfile("test")
	if err != nil {
		t.Fatal(err)
	}
	if fallback == first {
		t.Fatal("busy profile reused")
	}
	cleanup()
	if _, err := os.Stat(fallback); !os.IsNotExist(err) {
		t.Fatal("fallback left behind", err)
	}
	release()
	reused, releaseAgain, err := browserProfile("test")
	if err != nil {
		t.Fatal(err)
	}
	if reused != first {
		t.Fatal("profile not reused")
	}
	releaseAgain()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("LFM_TEST_CHROME_ARGS", args)
	tool := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nkill -0 -$$ || exit 2\nprintf '%s\\n' \"$@\" > \"$LFM_TEST_CHROME_ARGS\"\nprintf scratch > \"$TMPDIR/child-file\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := profileBrowser(context.Background(), BrowserOptions(tool), time.Second, "test"); err == nil {
		t.Fatal("expected launch failure")
	}
	raw, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--incognito", "--disable-component-update", "--disable-features=OptimizationGuideModelDownloading,OptimizationHints,OptimizationTargetPrediction", "--user-data-dir=" + first} {
		if !strings.Contains(string(raw), flag+"\n") {
			t.Errorf("missing %s: %s", flag, raw)
		}
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed browser left scratch: %v, %v", entries, err)
	}
	unlock, err := cache.Lock(first)
	if err != nil {
		t.Fatal("failed startup retained lock", err)
	}
	unlock()
}
