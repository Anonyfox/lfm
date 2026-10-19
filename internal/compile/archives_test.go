package compile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/zipx"
)

// strictArchives holds every archive a compile wrote, and every archive
// inside one (a .docx in an edition's ZIP), to zipx's rules: what EPUB
// validators, Word and LibreOffice demand of the bytes themselves.
func strictArchives(t *testing.T, dir string) {
	t.Helper()
	var check func(name string, data []byte)
	check = func(name string, data []byte) {
		for _, p := range zipx.Problems(data) {
			t.Errorf("%s: %s", name, p)
		}
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		for _, f := range z.File {
			if archive(f.Name) {
				rc, _ := f.Open()
				inner, _ := io.ReadAll(rc)
				rc.Close()
				check(name+"/"+f.Name, inner)
			}
		}
	}
	n := 0
	for _, root := range []string{"output", "distribution"} {
		filepath.WalkDir(filepath.Join(dir, root), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !archive(path) {
				return nil
			}
			data, _ := os.ReadFile(path)
			rel, _ := filepath.Rel(dir, path)
			check(rel, data)
			n++
			return nil
		})
	}
	if n == 0 {
		t.Error("the compile wrote no archive to check")
	}
}

func archive(name string) bool {
	for _, ext := range []string{".zip", ".epub", ".docx", ".pptx", ".xlsx"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
