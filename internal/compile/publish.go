package compile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Anonyfox/lfm/internal/project"
)

// publish replaces output/ with files. The new tree is written next to the old
// one and swapped in with renames, so readers never see a half-written folder
// and a failed write leaves the previous output untouched.
func publish(p project.Project, files map[string][]byte) error {
	output := p.Output()
	if info, err := os.Lstat(output); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("%s must be a real directory, not a file or symlink", output)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	removeStale(p.Root)
	next, err := os.MkdirTemp(p.Root, project.OutputDir+".next-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(next) // no-op after a successful swap
	if err := os.Chmod(next, 0o755); err != nil {
		return err
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(next, name), contents, 0o644); err != nil {
			return err
		}
	}
	previous := ""
	if _, err := os.Lstat(output); err == nil {
		previous = next + ".previous"
		if err := os.Rename(output, previous); err != nil {
			return err
		}
	}
	if err := os.Rename(next, output); err != nil {
		if previous != "" {
			if restore := os.Rename(previous, output); restore != nil {
				return fmt.Errorf("publish: %v; restoring the previous output failed: %v; it is at %s", err, restore, previous)
			}
		}
		return err
	}
	if previous != "" {
		// The new output is live. A leftover previous tree is removed by the
		// next publish and must not turn this success into a failure.
		os.RemoveAll(previous)
	}
	return nil
}

// removeStale deletes leftovers of publishes interrupted by a crash.
func removeStale(root string) {
	matches, _ := filepath.Glob(filepath.Join(root, project.OutputDir+".next-*"))
	for _, path := range matches {
		os.RemoveAll(path)
	}
}
