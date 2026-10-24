package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/pdfx"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/release"
)

func TestBookIsPublishedBehindItsPath(t *testing.T) {
	t.Parallel()
	dir := bookProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	listing := "# Listing: The Weekend Starter\n\n## Description\n" + strings.Repeat("Keep a starter alive on a working week. ", 8) +
		"\n\n## Short description\nKeep a starter alive.\n\n## Back cover\nThe jar smelled like nail polish.\n\n## Author\nAda bakes.\n\n## Keywords\n- sourdough schedule\n\n## Tags\n- sourdough\n\n## Categories\n- BISAC: CKB009000 Cooking\n- Thema: WBVD Bread\n- KDP: Baking\n\n## Prices\n- Ebook: 4.99 EUR\n\n## AI\nText: none\nImages: none\n"
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(book.ListingFile)), []byte(listing), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v\n%+v", err, result.Validation.Diagnostics)
	}
	if result.Book == nil || !result.Book.Complete || !strings.HasSuffix(result.Book.URL, "/"+bookPath+"/") {
		t.Fatalf("book summary %+v", result.Book)
	}
	out := filepath.Join(dir, "output", bookPath)
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !bytes.Contains(page, []byte("the-weekend-starter.epub")) || !bytes.Contains(page, []byte("noindex")) {
		t.Fatalf("download page: %v %s", err, page)
	}
	epub, err := os.ReadFile(filepath.Join(out, "the-weekend-starter.epub"))
	if err != nil || !bytes.HasPrefix(epub, []byte("PK")) {
		t.Fatalf("epub: %v", err)
	}
	// A complete book prints its PDF when Chrome is available.
	if _, browserErr := release.BrowserPath(); browserErr == nil {
		if _, err := os.Stat(filepath.Join(out, "the-weekend-starter.pdf")); err != nil {
			t.Fatal(err)
		}
	}
	main, _ := os.ReadFile(filepath.Join(dir, "output", "index.html"))
	if bytes.Contains(main, []byte(bookPath)) {
		t.Fatal("the page links the download path")
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution", "book", "the-weekend-starter.epub")); err != nil {
		t.Fatalf("store packages: %v", err)
	}
	if _, browserErr := release.BrowserPath(); browserErr == nil && !testing.Short() {
		if result, err := Compile(context.Background(), dir, Options{Publish: true, Release: true}); err != nil || result.Book.Pending {
			t.Fatalf("release: %v, pending %v", err, result.Book.Pending)
		}
		pdf, err := os.ReadFile(filepath.Join(out, "the-weekend-starter.pdf"))
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) || pdfx.StampOf(pdf) == "" {
			t.Fatalf("pdf: %v", err)
		}
		// Three short chapters stay below every printer's minimum: the README
		// still describes the paperback lfm printed.
		readme, _ := os.ReadFile(filepath.Join(dir, "distribution", "book", "README.md"))
		if !regexp.MustCompile(`5x8in trim, \d*[02468] pages`).Match(readme) || !bytes.Contains(readme, []byte("chapters start on right-hand pages")) {
			t.Fatalf("paperback in README:\n%s", readme)
		}
		interior, err := os.ReadFile(filepath.Join(dir, "distribution", "book", "paperback-interior.pdf"))
		if err != nil || pdfx.PageCount(interior)%2 != 0 || bytes.Contains(interior, []byte("/Annot")) || bytes.Contains(interior, []byte("/Outlines")) {
			t.Fatalf("paperback interior: %v, %d pages", err, pdfx.PageCount(interior))
		}
		// The next ordinary compile reuses the prints of the unchanged book.
		again, err := Compile(context.Background(), dir, Options{Publish: true})
		reused, _ := os.ReadFile(filepath.Join(out, "the-weekend-starter.pdf"))
		if err != nil || again.Book.Pending || !bytes.Equal(reused, pdf) || len(again.Book.Channels) == 0 {
			t.Fatalf("prints not reused: %v, pending %v", err, again.Book.Pending)
		}
	}
	filepath.WalkDir(filepath.Join(dir, "output"), func(path string, d os.DirEntry, err error) error {
		if strings.Contains(path, "stores") || strings.Contains(path, "paperback") {
			t.Errorf("a store file was deployed: %s", path)
		}
		return nil
	})
}

func TestPageThatLeaksTheBookPathFails(t *testing.T) {
	t.Parallel()
	dir := bookProject(t)
	tmpl, _ := os.ReadFile(filepath.Join(dir, "index.html.tmpl"))
	tmpl = bytes.Replace(tmpl, []byte("</body>"), []byte("<p>Your copy: example.com/"+bookPath+"/</p></body>"), 1)
	os.WriteFile(filepath.Join(dir, "index.html.tmpl"), tmpl, 0o644)
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err == nil {
		t.Fatal("compile passed although the page contains the download path")
	}
	found := false
	for _, d := range result.Validation.Diagnostics {
		found = found || d.Rule == book.RuleDelivery
	}
	if !found {
		t.Fatalf("no %s finding: %+v", book.RuleDelivery, result.Validation.Diagnostics)
	}
}

func TestUnfinishedBookPublishesThePresalePage(t *testing.T) {
	t.Parallel()
	dir := bookProject(t)
	os.Remove(filepath.Join(dir, book.Dir, "chapters", "03.md"))
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %+v", err, result.Validation.Diagnostics)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "output", bookPath))
	if len(entries) != 1 || entries[0].Name() != "index.html" {
		t.Fatalf("unfinished book published %v", entries)
	}
	page, _ := os.ReadFile(filepath.Join(dir, "output", bookPath, "index.html"))
	if !bytes.Contains(page, []byte("being written")) {
		t.Fatal("presale message missing")
	}
	if next := result.Book.Next(); next == nil || next.N != 3 {
		t.Fatalf("next chapter %+v", next)
	}
}

func TestInvalidBookKindUsesBookConfigRule(t *testing.T) {
	for _, kind := range []string{"poetry", ""} {
		t.Run(kind, func(t *testing.T) {
			dir := bookProject(t)
			changeConfig(t, dir, func(c map[string]any) {
				c["deliverables"].(map[string]any)["book"].(map[string]any)["kind"] = kind
			})
			r, err := Compile(context.Background(), dir, Options{})
			if err == nil {
				t.Fatal("invalid kind compiled")
			}
			found := false
			for _, d := range r.Validation.Diagnostics {
				if d.Path == "deliverables.book.kind" {
					found = true
					if d.Rule != book.RuleConfig || d.Location.File != "lfm.json" || d.Location.Line < 1 || !strings.Contains(d.Message, "deliverables.book.kind") {
						t.Fatal(d)
					}
				}
			}
			if !found {
				t.Fatal(r.Validation.Diagnostics)
			}
		})
	}
}

func TestShopBookWithoutListingAndPreview(t *testing.T) {
	dir := bookProject(t)
	changeConfig(t, dir, func(c map[string]any) {
		cfg := c["deliverables"].(map[string]any)["book"].(map[string]any)
		cfg["sell"] = "shops"
		cfg["formats"] = []string{"epub"}
	})
	os.Remove(filepath.Join(dir, book.ListingFile))
	r, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Book.Complete {
		t.Fatal(r.Book)
	}
	full := filepath.Join(dir, "distribution/book/the-weekend-starter.epub")
	if _, err := os.Stat(full); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, book.Dir, "chapters/01.md")
	text, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, append(text, []byte("\n[Later](03.md) and [Available](02.md).\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, book.Dir, "chapters/03.md"))
	r, err = Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Book.Preview != "distribution/book/preview/" || r.Book.Written != 2 {
		t.Fatal(r.Book)
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution/book/preview/the-weekend-starter-preview.epub")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Fatalf("complete book remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "output", bookPath)); !os.IsNotExist(err) {
		t.Fatalf("shop book has a download page: %v", err)
	}
	os.Remove(filepath.Join(dir, book.Dir, "chapters/01.md"))
	os.Remove(filepath.Join(dir, book.Dir, "chapters/02.md"))
	r, err = Compile(context.Background(), dir, Options{Publish: true})
	if err != nil || r.Book.Written != 0 || r.Book.Preview != "" {
		t.Fatal(err, r.Book)
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution/book/preview/the-weekend-starter-preview.epub")); !os.IsNotExist(err) {
		t.Fatalf("stale preview: %v", err)
	}
}

func TestPageNeeded(t *testing.T) {
	for _, tc := range []struct {
		r    Result
		want bool
	}{
		{Result{Starter: true, Book: &book.Summary{Sell: "shops"}}, false},
		{Result{Starter: true, Book: &book.Summary{Sell: "page"}}, true},
		{Result{Starter: false, Book: &book.Summary{Sell: "shops"}}, true},
		{Result{Starter: true, PageOffers: true, Book: &book.Summary{Sell: "shops"}}, true},
		{Result{Starter: true}, true},
	} {
		if got := tc.r.PageNeeded(); got != tc.want {
			t.Errorf("%+v: %v", tc.r, got)
		}
	}
}
