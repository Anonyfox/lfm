//go:build unix

package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBrowserLockProcess(t *testing.T) {
	if os.Getenv("LFM_LOCK_HELPER") == "1" {
		os.Setenv("LFM_BROWSER_LOCK", os.Getenv("LFM_TEST_BROWSER_LOCK"))
		unlock, err := AcquireBrowser(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		return
	}
	t.Setenv("LFM_BROWSER_LOCK", filepath.Join(t.TempDir(), "browser.lock"))
	unlock, err := AcquireBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	nested, err := AcquireBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nested()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBrowserLockProcess$")
	cmd.Env = append(os.Environ(), "LFM_LOCK_HELPER=1", "LFM_TEST_BROWSER_LOCK="+os.Getenv("LFM_BROWSER_LOCK"))

	log := filepath.Join(t.TempDir(), "stderr")
	f, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		raw, _ := os.ReadFile(log)
		if strings.Contains(string(raw), "waiting for the box-wide browser lock") {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("second process bypassed flock: %v (%s)", err, raw)
		case <-time.After(10 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not wait")
		}
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("lock not released")
	}
	raw, _ := os.ReadFile(log)
	if strings.Count(string(raw), "waiting for the box-wide browser lock") != 1 {
		t.Fatalf("wait message: %q", raw)
	}
}

func TestBrowserLockCancellation(t *testing.T) {
	// Hold an independent open file description, rather than a process reference.
	path := filepath.Join(t.TempDir(), "browser.lock")
	t.Setenv("LFM_BROWSER_LOCK", path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if unlock, err := AcquireBrowser(ctx); err == nil {
		unlock()
		t.Fatal("cancelled wait acquired lock")
	}
}
