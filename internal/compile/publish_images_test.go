package compile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/project"
)

func TestPublishImagesMigrateCopiesAndShareDistribution(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	p := project.Project{Root: t.TempDir()}
	files := map[string][]byte{"photo.webp": []byte("image"), "index.html": []byte("page")}
	output := filepath.Join(p.Root, "output")
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(output, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := publish(p, files); err != nil {
		t.Fatal(err)
	}
	if err := publishTo(p, "distribution/bundle", files); err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(filepath.Join(output, "photo.webp"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(p.Root, "distribution/bundle/photo.webp"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Fatal("output and distribution images are copies")
	}
	if !holds(output, files) {
		t.Fatal("warm publication not reused")
	}
}
