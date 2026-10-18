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
	return publishTo(p, project.OutputDir, files)
}

// publishTo replaces the folder dir of the project with files, the same way.
func publishTo(p project.Project, dir string, files map[string][]byte) error {
	output := filepath.Join(p.Root, dir)
	if info, err := os.Lstat(output); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("%s must be a real directory, not a file or symlink", output)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	removeStale(p.Root, dir)
	next, err := os.MkdirTemp(p.Root, dir+".next-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(next) // no-op after a successful swap
	if err := os.Chmod(next, 0o755); err != nil {
		return err
	}
	for name, contents := range files {
		target := filepath.Join(next, filepath.FromSlash(name))
		if dir := filepath.Dir(target); dir != next {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
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
func removeStale(root, dir string) {
	matches, _ := filepath.Glob(filepath.Join(root, dir+".next-*"))
	for _, path := range matches {
		os.RemoveAll(path)
	}
}

// unpublish removes a folder lfm published, such as the store packages of a
// book whose listing was removed.
func unpublish(p project.Project, dir string) error {
	return os.RemoveAll(filepath.Join(p.Root, dir))
}
