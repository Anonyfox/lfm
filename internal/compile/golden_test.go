package compile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenPages compiles the example project and lfm.cash itself and
// holds every file in output/ to the bytes it compiled to before.
func TestGoldenPages(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{"example": "../../testdata/example", "site": "../../site"} {
		dir := t.TempDir()
		if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(filepath.Join(dir, "output"))
		if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		files := map[string][]byte{}
		out := filepath.Join(dir, "output")
		filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(out, path)
			files[filepath.ToSlash(rel)], _ = os.ReadFile(path)
			return nil
		})
		golden.Files(t, "page-"+name, files)
	}
}
