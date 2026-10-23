package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/deliver"
)

func assertManifest(t *testing.T, r Result) {
	t.Helper()
	if len(r.Files) != len(r.Artifact) {
		t.Fatalf("manifest has %d files, artifact has %d", len(r.Files), len(r.Artifact))
	}
	if !slices.IsSortedFunc(r.Files, func(a, b File) int { return strings.Compare(a.Name, b.Name) }) {
		t.Fatal("manifest is not sorted")
	}
	seen := map[string]bool{}
	for _, f := range r.Files {
		data, ok := r.Artifact[f.Name]
		sum := sha256.Sum256(data)
		if !ok || seen[f.Name] || f.Bytes != len(data) || f.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("manifest entry disagrees with artifact: %+v", f)
		}
		seen[f.Name] = true
	}
}

func TestManifestIncludesFinalizedAndDeliveredFiles(t *testing.T) {
	t.Parallel()
	// A stylesheet emitted by optimize and downloads emitted after verify must
	// participate just like assemble's files, with output-relative names.
	b := builder{files: map[string]emitted{}}
	for name, data := range map[string]string{"styles-012345abcdef.css": "body{color:red}", "robots.txt": "User-agent: *\nAllow: /\n", "sitemap.xml": "sitemap"} {
		if err := b.emit(name, "test", []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	var r Result
	b.describe(&r, []byte("page"))
	downloadPage(&r, "secret-download-path", []byte("download page"), shipped{
		Files: []deliver.File{{Name: "book.epub"}}, Data: map[string][]byte{"book.epub": []byte("book")},
	})
	r.listFiles()
	assertManifest(t, r)
	if len(r.Files) != 6 {
		t.Fatal(r.Files)
	}
}
