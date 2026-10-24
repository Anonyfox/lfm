package scratch

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPruneKnownOwnedScratch(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	old := time.Now().Add(-7 * time.Hour)
	names := []string{"lfm-lighthouse-chrome-old", "lfm-office-old", "lfm-ooxml-old", "lfm-tests-old", "lfm-frames-012345abcdef", "lfm-lighthouse-012345abcdef.json", "lfm-libreoffice"}
	for _, name := range append(names, "lfm-perf", "lfm-final", "lfm-frames-not-a-hash", "lfm-lighthouse-wrong.json") {
		path := filepath.Join(os.TempDir(), name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	fresh := filepath.Join(os.TempDir(), "lfm-office-fresh")
	if err := os.Mkdir(fresh, 0o700); err != nil {
		t.Fatal(err)
	}
	Prune(os.TempDir())
	for _, name := range names {
		_, err := os.Stat(filepath.Join(os.TempDir(), name))
		if ownedInfo(t, fresh) && !os.IsNotExist(err) {
			t.Errorf("kept %s: %v", name, err)
		}
	}
	for _, name := range []string{"lfm-office-fresh", "lfm-perf", "lfm-final", "lfm-frames-not-a-hash", "lfm-lighthouse-wrong.json"} {
		if _, err := os.Stat(filepath.Join(os.TempDir(), name)); err != nil {
			t.Errorf("removed %s", name)
		}
	}
}

func ownedInfo(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return owned(info)
}

func TestCleanupOnSIGTERM(t *testing.T) {
	if os.Getenv("LFM_SCRATCH_HELPER") == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		defer Cleanup()
		path, err := Dir("office")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(path)
		<-ctx.Done()
		return
	}
	t.Setenv("TMPDIR", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCleanupOnSIGTERM$")
	cmd.Env = append(os.Environ(), "LFM_SCRATCH_HELPER=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() {
		t.Fatal("helper did not create scratch")
	}
	path := scanner.Text()
	if filepath.Dir(path) != os.TempDir() {
		t.Fatalf("unexpected scratch: %s", path)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("scratch remains: %v", err)
	}
}
