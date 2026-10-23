package compile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/project"
)

// publicFiles reads fixed-URL files through the same source boundary as assets.
// WalkDir does not follow directory symlinks; even skipped symlink targets must
// stay within the project. Public bytes never enter asset processing or checks.
func (b *builder) publicFiles() (map[string][]byte, error) {
	dir, err := b.project.OptionalDir(project.PublicDir)
	if err != nil || dir == "" {
		return nil, err
	}
	files := map[string][]byte{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir && !entry.IsDir() {
			return fmt.Errorf("%s must be a directory; move the file into public/", project.PublicDir)
		}
		if path != dir && strings.HasPrefix(entry.Name(), ".") && !(entry.IsDir() && entry.Name() == ".well-known") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		real, err := b.project.Path(path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			info, err = os.Stat(real)
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		raw, err := b.project.Read(path)
		if err != nil {
			return err
		}
		b.recordInput(real)
		rel, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = raw
		return nil
	})
	return files, err
}

// addPublic runs after delivery so every generated name is known. Generated
// download folders own their whole URL namespace, including not-yet-built files.
func (b *builder) addPublic(result *Result) error {
	if len(b.public) == 0 {
		return nil
	}
	reserved := make(map[string]bool, len(result.Artifact))
	for name := range result.Artifact {
		first, _, _ := strings.Cut(name, "/")
		reserved[strings.ToLower(first)] = true
	}
	names := make([]string, 0, len(b.public))
	for name := range b.public {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		first, _, _ := strings.Cut(name, "/")
		if reserved[strings.ToLower(first)] {
			return fmt.Errorf("public/%s: output filename collision with lfm's %q; move or rename the public file", name, first)
		}
	}
	for _, name := range names {
		result.Artifact[name] = b.public[name]
	}
	return nil
}
