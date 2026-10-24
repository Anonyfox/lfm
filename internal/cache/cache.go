// Package cache bounds persistent lfm artifacts in the user cache.
package cache

import (
	"os"
	"path/filepath"
	"slices"
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

// Prune keeps at most 128 entries used within 30 days.
func Prune(dir string) {
	entries, _ := os.ReadDir(dir)
	type file struct {
		name string
		date time.Time
	}
	var files []file
	for _, entry := range entries {
		if info, e := entry.Info(); e == nil {
			files = append(files, file{entry.Name(), info.ModTime()})
		}
	}
	slices.SortFunc(files, func(a, b file) int { return b.date.Compare(a.date) })
	for i, entry := range files {
		if i >= 128 || time.Since(entry.date) > 30*24*time.Hour {
			_ = os.RemoveAll(filepath.Join(dir, entry.name))
		}
	}
}

// Touch records use of an artifact.
func Touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}
