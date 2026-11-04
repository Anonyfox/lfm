// Package office runs LibreOffice headless to see a file the way another
// office app does: one soffice run converts a batch of files.
package office

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Anonyfox/lfm/internal/scratch"
)

// Find locates LibreOffice's soffice: LFM_SOFFICE (a path, or none to
// turn it off for lfm's own tests), PATH, or the usual install locations.
func Find() (string, error) {
	switch p := os.Getenv("LFM_SOFFICE"); p {
	case "none", "off":
		return "", errNotFound // turned off, as in CI
	case "":
	default:
		return p, nil
	}
	if p, err := exec.LookPath("soffice"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice", "/usr/bin/soffice", "/usr/lib/libreoffice/program/soffice"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errNotFound
}

var errNotFound = errors.New("LibreOffice is not installed")

// Missing reports whether err means LibreOffice is not there, which no
// release needs: its checks are a second opinion.
func Missing(err error) bool { return errors.Is(err, errNotFound) }

// Convert writes files to a private folder, converts them all in one run
// to the filter (such as "pdf" or a csv filter with options) and returns
// every file LibreOffice wrote, by name. A private profile keeps parallel
// runs and the user's own LibreOffice apart.
func Convert(ctx context.Context, soffice string, files map[string][]byte, filter string) (map[string][]byte, error) {
	dir, err := scratch.Dir("office")
	if err != nil {
		return nil, err
	}
	defer scratch.Remove(dir)
	out := filepath.Join(dir, "out")
	args := []string{"-env:UserInstallation=file://" + filepath.ToSlash(filepath.Join(dir, "profile")), "--headless", "--norestore",
		"--convert-to", filter, "--outdir", out}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
		args = append(args, path)
	}
	cmd := exec.CommandContext(ctx, soffice, args...)
	// C.UTF-8 provides Unicode filenames without requiring a generated regional locale.
	locale := "en_US.UTF-8"
	if runtime.GOOS == "linux" {
		locale = "C.UTF-8"
	}
	cmd.Env = append(os.Environ(), "LC_ALL="+locale)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start LibreOffice: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("LibreOffice: %w: %s", err, output.String())
		}
	case <-time.After(2 * time.Minute):
		cmd.Process.Kill()
		return nil, errors.New("LibreOffice did not finish within two minutes")
	}
	result := map[string][]byte{}
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(out, e.Name()))
		if err != nil {
			return nil, err
		}
		result[e.Name()] = data
	}
	return result, nil
}
