package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newProject(t *testing.T) Project {
	t.Helper()
	p, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenResolvesARealDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "file.txt", "x")
	if _, err := Open(filepath.Join(dir, "file.txt")); err == nil {
		t.Fatal("a file opened as a project")
	}
	if _, err := Open(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("a missing directory opened as a project")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip(err)
	}
	p, err := Open(link)
	real, _ := filepath.EvalSymlinks(dir)
	if err != nil || p.Root != real {
		t.Fatalf("symlinked root not resolved: %q %v", p.Root, err)
	}
}

func TestPathKeepsSourcesInsideTheProject(t *testing.T) {
	p := newProject(t)
	outside := t.TempDir()
	write(t, outside, "secret.txt", "secret")
	write(t, p.Root, "assets/a.css", "a{}")
	write(t, p.Root, "output/old.css", "stale")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(p.Root, "assets", "escape.txt")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(filepath.Join(p.Root, "output", "old.css"), filepath.Join(p.Root, "assets", "generated.css")); err != nil {
		t.Fatal(err)
	}
	for name, ok := range map[string]bool{
		"assets/a.css":                       true,
		"output/old.css":                     false,
		"../secret.txt":                      false,
		"assets/escape.txt":                  false,
		"assets/generated.css":               false,
		"assets/missing.css":                 false,
		filepath.Join(outside, "secret.txt"): false,
	} {
		_, err := p.Path(name)
		if (err == nil) != ok {
			t.Errorf("Path(%q) error = %v, want ok=%v", name, err, ok)
		}
	}
	if raw, err := p.Read("assets/a.css"); err != nil || string(raw) != "a{}" {
		t.Fatalf("Read: %q %v", raw, err)
	}
	write(t, p.Root, "output.next-123/x.css", "half-published")
	if _, err := p.Path("output.next-123/x.css"); err == nil {
		t.Fatal("an interrupted publish became a source")
	}
}

func TestOptionalDir(t *testing.T) {
	p := newProject(t)
	if dir, err := p.OptionalDir("partials"); dir != "" || err != nil {
		t.Fatalf("absent: %q %v", dir, err)
	}
	if err := os.Mkdir(filepath.Join(p.Root, "partials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if dir, err := p.OptionalDir("partials"); !strings.HasSuffix(dir, "partials") || err != nil {
		t.Fatalf("present: %q %v", dir, err)
	}
	if p.Rel(filepath.Join(p.Root, "a", "b.txt")) != "a/b.txt" || p.Output() != filepath.Join(p.Root, "output") {
		t.Fatal("Rel/Output")
	}
}
