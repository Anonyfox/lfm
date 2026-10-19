package compile

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
	if holds(output, files) {
		return nil // already published: rewriting would only churn
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	next, err := os.MkdirTemp(parent, filepath.Base(output)+".next-")
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

// holds reports whether dir holds exactly files: the same names, sizes and
// bytes, and nothing else. Sizes are compared before any file is read.
func holds(dir string, files map[string][]byte) bool {
	n := 0
	same := true
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		want, ok := files[filepath.ToSlash(rel)]
		info, ierr := d.Info()
		if !ok || !d.Type().IsRegular() || ierr != nil || info.Size() != int64(len(want)) {
			same = false
			return filepath.SkipAll
		}
		n++
		return nil
	})
	if err != nil || !same || n != len(files) {
		return false
	}
	for name, want := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		// Stamped files carry the key of their inputs: the same key and
		// size are the same file, without reading it.
		if key := stampOf(want[max(0, len(want)-4096):]); key != "" && key == tailStampOfFile(path, stampOf) {
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
	}
	return true
}

// removeStale deletes leftovers of publishes interrupted by a crash.
func removeStale(root, dir string) {
	matches, _ := filepath.Glob(filepath.Join(root, dir+".next-*"))
	for _, path := range matches {
		os.RemoveAll(path)
	}
}

// units are the sellable units that own a folder in distribution/.
var units = map[string]bool{"book": true, "printable": true, "bundle": true, "spreadsheet": true, "document": true, "deck": true}

// publishDistribution replaces each unit's folder in distribution/ the way
// publish replaces output/, removes the units that have none any more, and
// clears anything else lfm no longer writes there, such as the flat layout of
// earlier versions. A distribution/ left empty is removed.
func publishDistribution(p project.Project, packages map[string]map[string][]byte) error {
	if packages == nil {
		return nil
	}
	root := filepath.Join(p.Root, project.DistributionDir)
	for unit, files := range packages {
		dir := filepath.Join(project.DistributionDir, unit)
		if len(files) == 0 {
			if err := os.RemoveAll(filepath.Join(p.Root, dir)); err != nil {
				return err
			}
			continue
		}
		if err := publishTo(p, dir, files); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if !units[e.Name()] && !strings.Contains(e.Name(), ".next-") {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
				return err
			}
		}
	}
	if entries, _ := os.ReadDir(root); len(entries) == 0 {
		os.Remove(root)
	}
	return nil
}
