package serve

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/project"
)

type stamp struct {
	Size  int64
	Time  int64
	Mode  fs.FileMode
	Hash  [32]byte
	Error string
}

type monitor struct {
	root         string
	dependencies map[string]bool
	cache        map[string]stamp
}

func newMonitor(root string) *monitor {
	return &monitor{root: root, dependencies: map[string]bool{}, cache: map[string]stamp{}}
}

// Polling works across native OSes, mounted shares and editor atomic replacements,
// with no platform processes or watcher limits. A file is read and hashed only
// when its size, mode or modification time changed, or while that time is
// recent enough that a coarse-grained file system could hide a same-size edit.
// stamp describes one file. ok is false for files that are not regular.
func (m *monitor) stamp(path string) (s stamp, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		return stamp{Error: err.Error()}, true
	}
	if !info.Mode().IsRegular() {
		return stamp{}, false
	}
	s = stamp{Size: info.Size(), Time: info.ModTime().UnixNano(), Mode: info.Mode()}
	settled := time.Since(info.ModTime()) > 2*time.Second
	if old, cached := m.cache[path]; cached && old.Error == "" && old.Size == s.Size && old.Time == s.Time && old.Mode == s.Mode && settled {
		s.Hash = old.Hash
		return s, true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		s.Error = err.Error()
	} else {
		s.Hash = sha256.Sum256(raw)
	}
	return s, true
}

func (m *monitor) snapshot() map[string]stamp {
	current := map[string]stamp{}
	add := func(path string) {
		if s, ok := m.stamp(path); ok {
			current[path] = s
		}
	}
	err := filepath.WalkDir(m.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			current[path] = stamp{Error: err.Error()}
			return nil
		}
		relative, _ := filepath.Rel(m.root, path)
		if entry.IsDir() {
			if path != m.root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || relative == project.OutputDir || strings.HasPrefix(relative, project.OutputDir+".next-") ||
				relative == project.DistributionDir || strings.HasPrefix(relative, project.DistributionDir+".next-")) {
				return filepath.SkipDir
			}
			return nil
		}
		if transient(entry.Name()) {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			// Compiler paths must remain within the project; never crawl symlink trees.
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				current[path] = stamp{Error: err.Error()}
				return nil
			}
			rel, err := filepath.Rel(m.root, real)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return nil
			}
		}
		add(path)
		return nil
	})
	if err != nil {
		current[m.root] = stamp{Error: err.Error()}
	}
	for path := range m.dependencies {
		add(path)
	}
	// Detect package-manager replacements without traversing installed packages.
	modules := filepath.Join(m.root, "node_modules")
	if entries, err := os.ReadDir(modules); err == nil {
		for _, entry := range entries {
			path := filepath.Join(modules, entry.Name())
			info, err := entry.Info()
			if err == nil {
				current[path+string(os.PathSeparator)] = stamp{Time: info.ModTime().UnixNano(), Mode: info.Mode()}
			}
			if strings.HasPrefix(entry.Name(), "@") {
				if children, err := os.ReadDir(path); err == nil {
					for _, child := range children {
						if info, err := child.Info(); err == nil {
							current[filepath.Join(path, child.Name())+string(os.PathSeparator)] = stamp{Time: info.ModTime().UnixNano(), Mode: info.Mode()}
						}
					}
				}
			}
		}
	}
	m.cache = current
	return current
}

// include watches resolved package files and returns the ones that are new.
func (m *monitor) include(inputs []string) (added []string) {
	for _, path := range inputs {
		rel, err := filepath.Rel(m.root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if !strings.Contains(filepath.ToSlash(rel), "node_modules/") {
			continue
		}
		watch := []string{path}
		// Resolution can change through package.json, even when source bytes don't.
		for parent := filepath.Dir(path); parent != m.root; parent = filepath.Dir(parent) {
			watch = append(watch, filepath.Join(parent, "package.json"))
			if parent == filepath.Dir(parent) {
				break
			}
		}
		for _, p := range watch {
			if !m.dependencies[p] {
				m.dependencies[p] = true
				added = append(added, p)
			}
		}
	}
	return added
}

func changed(a, b map[string]stamp) bool {
	if len(a) != len(b) {
		return true
	}
	for path, s := range a {
		if other, ok := b[path]; !ok || s.Hash != other.Hash || s.Size != other.Size || s.Mode != other.Mode || s.Error != other.Error {
			return true
		}
	}
	// Directory entries use timestamps to detect package installation/replacement.
	for path, s := range a {
		if strings.HasSuffix(path, string(os.PathSeparator)) && b[path].Time != s.Time {
			return true
		}
	}
	return false
}

func transient(name string) bool {
	return name == ".DS_Store" || name == "Thumbs.db" || name == "4913" || strings.HasSuffix(name, "~") || strings.HasPrefix(name, ".#") || strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".swo") || strings.HasSuffix(name, ".tmp")
}
