//go:build linux

package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLowerPriority(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"nice", "ionice"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	cmd := &exec.Cmd{Path: "/chrome", Args: []string{"/chrome", "--headless"}}
	lowerPriority(cmd)
	want := []string{filepath.Join(dir, "nice"), "-n", "10", filepath.Join(dir, "ionice"), "-c", "2", "-n", "7", "/chrome", "--headless"}
	if !reflect.DeepEqual(cmd.Args, want) || cmd.Path != want[0] {
		t.Fatalf("%q %q", cmd.Path, cmd.Args)
	}
	t.Setenv("PATH", t.TempDir())
	cmd = &exec.Cmd{Path: "/chrome", Args: []string{"/chrome", "--headless"}}
	lowerPriority(cmd)
	if cmd.Path != "/chrome" || !reflect.DeepEqual(cmd.Args, []string{"/chrome", "--headless"}) {
		t.Fatal(cmd)
	}
}
