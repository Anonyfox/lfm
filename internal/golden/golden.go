// Package golden holds tests to the exact output they produced before: a
// manifest of every file, and of every entry inside an archive, with its
// size and content hash, kept in testdata/golden. A refactor that changes
// one byte a buyer would get fails the test; LFM_UPDATE_GOLDEN=1 rewrites
// the manifests when the change is meant.
package golden

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Files compares files, by name, with the manifest testdata/golden/<name>.txt.
// Archives (.zip, .epub, .docx, .pptx, .xlsx) are listed entry by entry,
// their comments, which carry build stamps, left out.
func Files(t testing.TB, name string, files map[string][]byte) {
	t.Helper()
	var lines []string
	for _, n := range sorted(files) {
		lines = append(lines, manifest(n, files[n])...)
	}
	got := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("testdata", "golden", name+".txt")
	if os.Getenv("LFM_UPDATE_GOLDEN") != "" {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no golden manifest %s; run with LFM_UPDATE_GOLDEN=1 once", path)
	}
	if got != string(want) {
		t.Errorf("output differs from %s; if the change is meant, run with LFM_UPDATE_GOLDEN=1:\n%s", path, diff(string(want), got))
	}
}

func manifest(name string, data []byte) []string {
	if archive(name) {
		if z, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			var out []string
			for _, f := range z.File {
				rc, err := f.Open()
				if err != nil {
					out = append(out, name+"/"+f.Name+" unreadable")
					continue
				}
				inner, _ := io.ReadAll(rc)
				rc.Close()
				out = append(out, manifest(name+"/"+f.Name, inner)...)
			}
			return out
		}
	}
	sum := sha256.Sum256(data)
	return []string{fmt.Sprintf("%s %d %s", name, len(data), hex.EncodeToString(sum[:8]))}
}

func archive(name string) bool {
	for _, ext := range []string{".zip", ".epub", ".docx", ".pptx", ".xlsx"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func sorted(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// diff lists the lines only one side has.
func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	in := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	inW, inG := in(w), in(g)
	var b strings.Builder
	n := 0
	for _, x := range w {
		if !inG[x] && x != "" && n < 40 {
			fmt.Fprintf(&b, "- %s\n", x)
			n++
		}
	}
	for _, x := range g {
		if !inW[x] && x != "" && n < 80 {
			fmt.Fprintf(&b, "+ %s\n", x)
			n++
		}
	}
	return b.String()
}
