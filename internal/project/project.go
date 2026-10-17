// Package project resolves an lfm project on disk: its fixed layout, the
// boundary that every source read must stay inside, and its configuration.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The fixed layout. Everything else inside the root is free-form; assets/ is a
// convention, not a requirement.
const (
	ConfigFile   = "lfm.json"
	TemplateFile = "index.html.tmpl"
	PartialsDir  = "partials"
	DataDir      = "data"
	OutputDir    = "output"
	ResearchDir  = "research" // durable research files; never a build input
)

// Project is a resolved project root. The zero value is not usable.
type Project struct {
	Root string // absolute, symlinks resolved
}

// Open resolves dir to a real directory.
func Open(dir string) (Project, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return Project{}, fmt.Errorf("%s: no such directory", dir)
	}
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Project{}, err
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("%s is not a directory", dir)
	}
	return Project{Root: root}, nil
}

// Output is the directory a successful compile replaces.
func (p Project) Output() string { return filepath.Join(p.Root, OutputDir) }

// Rel returns a slash-separated path relative to the root.
func (p Project) Rel(path string) string {
	rel, err := filepath.Rel(p.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Path resolves name (relative to the root, or absolute) to a real source file
// path. Sources must stay inside the root and outside output/ (and the
// output.next-* folders of a publish in progress), also after following
// symlinks, so generated files can never feed back into a build.
func (p Project) Path(name string) (string, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.Root, path)
	}
	if !p.permitted(path) {
		return "", fmt.Errorf("%s: sources must stay inside the project and outside %s/", name, OutputDir)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if !p.permitted(real) {
		return "", fmt.Errorf("%s: symlink leaves the project", name)
	}
	return real, nil
}

func (p Project) permitted(path string) bool {
	rel, err := filepath.Rel(p.Root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return first != OutputDir && !strings.HasPrefix(first, OutputDir+".next-")
}

// Read reads a contained source file.
func (p Project) Read(name string) ([]byte, error) {
	path, err := p.Path(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// OptionalDir resolves a contained directory that may be absent; absent yields "".
func (p Project) OptionalDir(name string) (string, error) {
	if _, err := os.Lstat(filepath.Join(p.Root, name)); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return p.Path(name)
}
