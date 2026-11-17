package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/cache"
	"github.com/chromedp/chromedp/remote"
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

func TestProfileSingletonCleanupRequiresOwnership(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory locks")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	profile, release, err := browserProfile("singletons")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie", "keep"} {
		if err := os.Symlink("missing-target", filepath.Join(profile, name)); err != nil {
			t.Fatal(err)
		}
	}
	fallback, cleanup, err := browserProfile("singletons")
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if fallback == profile {
		t.Fatal("used locked profile")
	}
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if _, err := os.Lstat(filepath.Join(profile, name)); err != nil {
			t.Fatal("modified locked profile", err)
		}
	}
	release()
	_, unlock, err := browserProfile("singletons")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if _, err := os.Lstat(filepath.Join(profile, name)); !os.IsNotExist(err) {
			t.Fatal("stale singleton remains", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(profile, "keep")); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserStartupBound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LFM_BROWSER_LOCK", filepath.Join(t.TempDir(), "browser.lock"))
	for _, websocket := range []bool{false, true} {
		for _, script := range []string{"exit 1", "exec sleep 30"} {
			t.Run(fmt.Sprintf("websocket=%t/%s", websocket, script), func(t *testing.T) {
				tool := filepath.Join(t.TempDir(), "chrome")
				if err := os.WriteFile(tool, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				opts := BrowserOptions(tool)
				if websocket {
					opts = append(opts, remote.WebSocket)
				}
				_, _, err := profileBrowser(context.Background(), opts, time.Second, "critical")
				if err == nil || !strings.Contains(err.Error(), "critical CSS: Chrome startup") || !strings.Contains(err.Error(), "chrome-critical") || !strings.Contains(err.Error(), "LFM_CHROME") {
					t.Fatalf("%v", err)
				}
				if script == "exec sleep 30" && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("silent browser did not hit startup deadline: %v", err)
				}
				if time.Since(start) > 3*time.Second {
					t.Fatal("startup exceeded bound", time.Since(start))
				}
				profile, unlock, err := browserProfile("critical")
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
				if profile != cache.Dir("chrome-critical") {
					t.Fatal("startup retained profile lock")
				}
			})
		}
	}
	// A busy job slot is a queue, not a failure: the launch waits, says so on
	// stderr, and only the caller's context ends the wait.
	log, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	original, notice := os.Stderr, jobWaitNotice
	os.Stderr, jobWaitNotice = log, 50*time.Millisecond
	defer func() { os.Stderr, jobWaitNotice = original, notice }()
	jobs := WithBrowserJobs(context.Background(), 1)
	release, err := browserJob(jobs)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(jobs, 300*time.Millisecond)
	defer cancel()
	_, _, err = profileBrowser(ctx, BrowserOptions("unused"), time.Second, "critical")
	if err == nil || !strings.Contains(err.Error(), "wait for browser job slot") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("admission wait: %v", err)
	}
	body, _ := os.ReadFile(log.Name())
	if !strings.Contains(string(body), "lfm: still waiting for a browser job slot (1 of 1 busy") {
		t.Fatalf("no wait notice: %q", body)
	}
}

func TestBrowserProgressStderr(t *testing.T) {
	log, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	original := os.Stderr
	os.Stderr = log
	defer func() { os.Stderr = original }()
	browserProgress("critical", "/cache/chrome-critical")
	body, err := os.ReadFile(log.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "lfm: critical CSS: starting Chrome (profile /cache/chrome-critical)\n" {
		t.Fatalf("%q", body)
	}
}
