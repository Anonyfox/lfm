package release

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLighthouseLockWaitsAndSurvivesHolderCrash(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("advisory flock requires Unix")
	}
	if os.Getenv("LFM_TEST_LOCK_HOLDER") == "1" {
		// TestMain gives each process its own cache; restore the shared one.
		t.Setenv("XDG_CACHE_HOME", os.Getenv("LFM_TEST_LOCK_CACHE"))
		t.Setenv("HOME", os.Getenv("LFM_TEST_LOCK_HOME"))
		unlock, err := lighthouseLock(context.Background(), io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		fmt.Println("locked")
		time.Sleep(time.Minute)
		return
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLighthouseLockWaitsAndSurvivesHolderCrash$")
	child.Env = append(os.Environ(), "LFM_TEST_LOCK_HOLDER=1", "LFM_TEST_LOCK_CACHE="+os.Getenv("XDG_CACHE_HOME"), "LFM_TEST_LOCK_HOME="+os.Getenv("HOME"))
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if scanner := bufio.NewScanner(out); !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatal("holder did not acquire lock", scanner.Err())
	}
	wait, stop := context.WithTimeout(ctx, 300*time.Millisecond)
	defer stop()
	var progress bytes.Buffer
	unlock, err := lighthouseLock(wait, &progress)
	want := "waiting for Lighthouse lock (held by pid " + strconv.Itoa(child.Process.Pid) + ")"
	if unlock != nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(progress.String(), want) || !strings.Contains(err.Error(), "wait capped at 10m") {
		t.Fatalf("unlock=%v err=%v progress=%q", unlock != nil, err, &progress)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	unlock, err = lighthouseLock(ctx, io.Discard)
	if err != nil {
		t.Fatal("crash retained lock:", err)
	}
	unlock()
}

func TestLighthouseTimeoutUsesLoadOverrideAndParentCause(t *testing.T) {
	for _, tc := range []struct {
		override string
		load     float64
		want     time.Duration
	}{
		{"", 0, 45 * time.Second},
		{"", 8, 45 * time.Second},
		{"", 16, 90 * time.Second},
		{"", 80, 5 * time.Minute},
		{"90s", 80, 90 * time.Second},
		{"1ms", 0, time.Millisecond},
		{"0", 0, 0},
		{"-1s", 0, 0},
		{"oops", 0, 0},
	} {
		got, err := lighthouseBudget(tc.override, tc.load, 8)
		if got != tc.want || (err != nil) != (tc.want == 0) {
			t.Fatalf("%+v: %s, %v", tc, got, err)
		}
	}
	t.Setenv("LFM_LIGHTHOUSE_TIMEOUT", "1ms")
	ctx, cancel, err := lighthouseContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	<-ctx.Done()
	if cause := context.Cause(ctx); !errors.Is(cause, context.DeadlineExceeded) || !strings.Contains(cause.Error(), "host load") || !strings.Contains(cause.Error(), "LFM_LIGHTHOUSE_TIMEOUT") {
		t.Fatal(cause)
	}
	parent, abort := context.WithCancelCause(context.Background())
	cause := errors.New("caller stopped")
	abort(cause)
	ctx, cancel, err = lighthouseContext(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if context.Cause(ctx) != cause {
		t.Fatal("parent cause replaced:", context.Cause(ctx))
	}
}
