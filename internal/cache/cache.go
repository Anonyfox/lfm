// Package cache bounds persistent lfm artifacts in the user cache.
package cache

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Dir returns a cache category, falling back to the system temp directory.
func Dir(category string) string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "lfm", category)
}

// Prune expires artifacts after two days and evicts the least recently used
// entries until the whole lfm cache fits in 300 MiB. Directories are artifacts
// (frame sets and profiles), never followed through symlinks.
func Prune() {
	root := Dir("")
	type artifact struct {
		path   string
		date   time.Time
		size   int64
		unlock func()
		busy   bool
	}
	var files []artifact
	var total int64
	categories, _ := os.ReadDir(root)
	for _, category := range categories {
		path := filepath.Join(root, category.Name())
		entries := []fs.DirEntry{category}
		if category.IsDir() && !strings.HasPrefix(category.Name(), "chrome-") {
			entries, _ = os.ReadDir(path)
		} else {
			path = root
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			f := artifact{path: filepath.Join(path, entry.Name()), date: info.ModTime()}
			if strings.HasPrefix(category.Name(), "chrome-") && entry.IsDir() {
				f.unlock, err = Lock(f.path)
				f.busy = err != nil // A running Chrome owns this profile.
			}
			_ = filepath.WalkDir(f.path, func(_ string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if i, e := d.Info(); e == nil {
						if shared(i) {
							f.busy = true
						} else {
							f.size += i.Size()
						}
					}
				}
				return nil
			})
			total += f.size
			files = append(files, f)
		}
	}
	slices.SortFunc(files, func(a, b artifact) int { return a.date.Compare(b.date) })
	for _, f := range files {
		if !f.busy && (time.Since(f.date) > 2*24*time.Hour || total > 300<<20) {
			if os.RemoveAll(f.path) == nil {
				total -= f.size
			}
		}
		if f.unlock != nil {
			f.unlock()
		}
	}
}

// Touch records use of an artifact.
func Touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}
