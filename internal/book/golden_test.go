package book

import (
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/golden"
)

// TestGoldenOutput holds the EPUB, the print documents and their assets
// to the bytes they had before.
func TestGoldenOutput(t *testing.T) {
	p, voices, peers := fixture(t)
	r := Check(p, testConfig, true, nil, "https://example.org/sourdough/", voices, peers)
	built, err := Build(r, func(rel string) ([]byte, error) { return p.Read(Dir + "/" + rel) }, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"book.epub":       built.EPUB,
		"print.html":      built.Print,
		"paperback.html":  built.Document(PrintOptions{Paper: true, Inside: 0.75}),
		"cover-wrap.html": built.CoverDocument(0.5, true),
	}
	for name, data := range built.PrintAssets {
		files["assets/"+name] = data
	}
	golden.Files(t, "book-sourdough", files)
}
