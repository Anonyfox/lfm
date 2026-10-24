// Package scratch owns temporary working directories and stale temp cleanup.
package scratch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var state struct {
	sync.Mutex
	paths map[string]bool
}
var once sync.Once
var legacy = regexp.MustCompile(`^(lfm-frames-[0-9a-f]{12}|lfm-lighthouse-[0-9a-f]{12}\.json|lfm-libreoffice)$`)

// Prune removes only owned, known scratch names in dir older than six hours.
func Prune(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		name := entry.Name()
		known := legacy.MatchString(name)
		for _, prefix := range []string{"lfm-lighthouse-chrome-", "lfm-office-", "lfm-ooxml-", "lfm-tests-"} {
			known = known || strings.HasPrefix(name, prefix)
		}
		if !known {
			continue
		}
		info, err := entry.Info()
		if err == nil && owned(info) && time.Since(info.ModTime()) > 6*time.Hour {
			_ = os.RemoveAll(filepath.Join(dir, name))
		}
	}
}

// Start prunes the system temp directory once per process.
func Start() { once.Do(func() { Prune(os.TempDir()) }) }

// Dir creates and registers an ephemeral directory in the system temp
// directory.
func Dir(prefix string) (string, error) { return DirIn("", prefix) }

// DirIn creates and registers an ephemeral directory in parent ("" for the
// system temp directory).
func DirIn(parent, prefix string) (string, error) {
	Start()
	path, err := os.MkdirTemp(parent, "lfm-"+prefix+"-")
	if err == nil {
		register(path)
	}
	return path, err
}

// File registers an atomic-write temporary file beside its destination.
func File(dir, pattern string) (*os.File, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err == nil {
		register(f.Name())
	}
	return f, err
}

func register(path string) {
	state.Lock()
	defer state.Unlock()
	if state.paths == nil {
		state.paths = make(map[string]bool)
	}
	state.paths[path] = true
}

// Remove removes a registered path, retrying while helpers release files.
func Remove(path string) error {
	var err error
	for i := 0; i < 10; i++ {
		err = os.RemoveAll(path)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err == nil {
		state.Lock()
		delete(state.paths, path)
		state.Unlock()
	}
	return err
}

// Cleanup removes all paths still registered by this process.
func Cleanup() {
	state.Lock()
	paths := make([]string, 0, len(state.paths))
	for path := range state.paths {
		paths = append(paths, path)
	}
	state.Unlock()
	for _, path := range paths {
		_ = Remove(path)
	}
}
