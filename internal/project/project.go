// Package project resolves an lfm project on disk: its fixed layout, the
// boundary that every source read must stay inside, and its configuration.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The fixed layout. Everything else inside the root is free-form; assets/ is a
// convention, not a requirement.
const (
	ConfigFile   = "lfm.json"
	TemplateFile = "index.html.tmpl"
	PartialsDir  = "partials"
	DataDir      = "data"
	PublicDir    = "public"
	OutputDir    = "output"
	// DistributionDir holds the store packages of deliverables: never deployed,
	// because its files are what buyers pay for.
	DistributionDir = "distribution"
	ResearchDir     = "research" // durable research files; never a build input
)

// Project is a resolved project root. The zero value is not usable.
type Project struct {
	Root     string    // absolute, symlinks resolved
	resolved *sync.Map // Path results
	reals    *sync.Map // real paths of files and directories below Root
}

// resolution is a memoized Path result. Open a Project per compile: it lives for one compile, so
// every source path costs its symlink walk once instead of once per reference.
type resolution struct {
	path string
	err  error
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
	return Project{Root: root, resolved: &sync.Map{}, reals: &sync.Map{}}, nil
}

// ErrNoProject means a directory holds neither lfm.json nor the template.
var ErrNoProject = errors.New("not an lfm project")

// Expect reports ErrNoProject, with what to do, when the root holds neither
// lfm.json nor index.html.tmpl. A project missing only one of them is still
// a project: the compile reports the missing file as a finding.
func (p Project) Expect(dir string) error {
	for _, name := range []string{ConfigFile, TemplateFile} {
		if _, err := os.Lstat(filepath.Join(p.Root, name)); err == nil {
			return nil
		}
	}
	init := "lfm init"
	if dir != "." {
		init += " " + dir
	}
	return fmt.Errorf("%w: %s has no %s or %s; create one with '%s'", ErrNoProject, dir, ConfigFile, TemplateFile, init)
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
	if p.resolved == nil {
		return p.resolve(name, path)
	}
	if r, ok := p.resolved.Load(path); ok {
		return r.(resolution).path, r.(resolution).err
	}
	real, err := p.resolve(name, path)
	p.resolved.Store(path, resolution{real, err})
	return real, err
}

func (p Project) resolve(name, path string) (string, error) {
	if !p.permitted(path) {
		return "", fmt.Errorf("%s: sources must stay inside the project and outside %s/", name, OutputDir)
	}
	real, err := p.real(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if !p.permitted(real) {
		return "", fmt.Errorf("%s: symlink leaves the project", name)
	}
	return real, nil
}

// real is path with every symlink resolved, path inside the root. Each
// directory is resolved once per project and shared by everything in it,
// so a file costs one Lstat rather than one per path component.
func (p Project) real(path string) (string, error) {
	if path == p.Root {
		return p.Root, nil
	}
	if p.reals != nil {
		if r, ok := p.reals.Load(path); ok {
			return r.(resolution).path, r.(resolution).err
		}
	}
	parent, err := p.real(filepath.Dir(path))
	var real string
	if err == nil {
		candidate := filepath.Join(parent, filepath.Base(path))
		var info os.FileInfo
		if info, err = os.Lstat(candidate); err == nil {
			real = candidate
			if info.Mode()&os.ModeSymlink != 0 {
				real, err = filepath.EvalSymlinks(candidate)
			}
		}
	}
	if p.reals != nil {
		p.reals.Store(path, resolution{real, err})
	}
	return real, err
}

func (p Project) permitted(path string) bool {
	rel, err := filepath.Rel(p.Root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return first != OutputDir && !strings.HasPrefix(first, OutputDir+".next-") &&
		first != DistributionDir && !strings.HasPrefix(first, DistributionDir+".next-")
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
