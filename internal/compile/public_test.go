package compile

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPublicFilesAreVerbatimPublishedInputs(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	files := map[string]string{
		"abc.txt":                  "verification-key\r\n",
		"nested/more/file.bin":     "\x00\xff\r\n",
		"nested/other.txt":         "another file",
		".well-known/security.txt": "Contact: mailto:security@example.com\n",
		"nested/.well-known/key":   "nested key",
		"favicon.ico":              "verbatim icon bytes",
		"styles.css":               "unparsed {{ invalid css",
	}
	for name, contents := range files {
		write(t, root, "public/"+name, contents)
	}
	skipped := []string{".env", ".git/config", "nested/.secret", "nested/.cache/file", ".well-known/.private", ".well-known/.hidden/file"}
	for _, name := range skipped {
		write(t, root, "public/"+name, "must not be deployed")
	}
	r, err := Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	for name, want := range files {
		if got := string(r.Artifact[name]); got != want || read(t, root, "output/"+name) != want {
			t.Errorf("%s: public bytes changed: %q", name, got)
		}
		real, err := filepath.EvalSymlinks(filepath.Join(root, "public", filepath.FromSlash(name)))
		if err != nil || !slices.Contains(r.Inputs, real) {
			t.Errorf("%s missing from inputs: %v, %v", name, r.Inputs, err)
		}
	}
	for _, name := range skipped {
		if _, ok := r.Artifact[name]; ok {
			t.Errorf("dotfile %s was deployed", name)
		}
	}
	if _, ok := r.Artifact["favicon.png"]; !ok || !strings.Contains(string(r.Artifact["index.html"]), "favicon.png") {
		t.Fatal("public favicon.ico changed the generated favicon")
	}
	assertManifest(t, r)
	if err := os.Remove(filepath.Join(root, "public/abc.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "public/.well-known/security.txt", "updated contact")
	r, err = Compile(context.Background(), root, Options{Publish: true})
	if err != nil {
		t.Fatal(err, r.Validation)
	}
	if _, err := os.Stat(filepath.Join(root, "output/abc.txt")); !os.IsNotExist(err) {
		t.Fatal("removed public file survived publish", err)
	}
	if got := read(t, root, "output/.well-known/security.txt"); got != "updated contact" {
		t.Fatal("public update did not survive publish", got)
	}
}

func TestPublicCollisionsKeepPreviousOutput(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"index.html", "INDEX.HTML", "index.html/child.txt", "favicon.png", "picture.svg", "picture.svg/child.txt", bookPath, bookPath + "/index.html", bookPath + "/extra.txt"} {
		t.Run(name, func(t *testing.T) {
			var root string
			if strings.HasPrefix(name, bookPath) {
				root = bookProject(t)
			} else {
				root = fixture(t)
			}
			write(t, root, "output/previous.txt", "previous output")
			write(t, root, "public/"+name, "collision")
			r, err := Compile(context.Background(), root, Options{Publish: true})
			if err == nil || r.Published || r.Errors() == 0 {
				t.Fatal("public collision accepted", err, r.Validation)
			}
			found := false
			for _, d := range r.Validation.Diagnostics {
				if d.Rule == "build.failed" && strings.Contains(d.Message, "public/"+name) && strings.Contains(d.Message, "move or rename") {
					found = true
				}
			}
			if !found {
				t.Fatal("missing collision diagnostic naming the file and fix", r.Validation)
			}
			if got := read(t, root, "output/previous.txt"); got != "previous output" {
				t.Fatal("failed compile replaced previous output")
			}
		})
	}
}

func TestPublicSymlinkBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, link, target string }{
		{"file escape", "public/key.txt", "outside/key.txt"},
		{"directory escape", "public/keys", "outside"},
		{"root escape", "public", "outside"},
		{"output feedback", "public/key.txt", "output/key.txt"},
		{"distribution feedback", "public/key.txt", "distribution/key.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			targetRoot := root
			if strings.HasPrefix(tc.target, "outside") {
				targetRoot = t.TempDir()
			}
			write(t, targetRoot, "outside/key.txt", "private")
			write(t, root, "output/key.txt", "private output")
			write(t, root, "distribution/key.txt", "private download")
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, tc.link)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(targetRoot, tc.target), filepath.Join(root, tc.link)); err != nil {
				t.Fatal(err)
			}
			r, err := Compile(context.Background(), root, Options{})
			if err == nil || !strings.Contains(err.Error(), "symlink leaves the project") || r.Errors() == 0 {
				t.Fatal("public symlink escape accepted", err, r.Validation)
			}
		})
	}
	t.Run("contained file", func(t *testing.T) {
		root := fixture(t)
		write(t, root, "keys/source.txt", "contained key")
		if err := os.Mkdir(filepath.Join(root, "public"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "keys/source.txt"), filepath.Join(root, "public/key.txt")); err != nil {
			t.Fatal(err)
		}
		r, err := Compile(context.Background(), root, Options{})
		if err != nil || string(r.Artifact["key.txt"]) != "contained key" {
			t.Fatal("contained public symlink rejected", err, r.Validation)
		}
	})
}
