package compile

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/pdfx"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

const bookPath = "q7w2k9m4x8c3v6b1n5zt"

const bookConcept = `# Book: Starter rescue

## Reader
Home bakers whose starter died twice.
Not for: professional bakers.

## Promise
After three weekends the reader keeps a starter alive.
Won't cover: enriched doughs.

## Big idea
Feed the schedule, not the jar.

## Hooks
- Title: "The Weekend Starter"
- Subtitle: "Keep sourdough alive on a working week"
- Opening: the jar that smelled like nail polish.

## Voice
- Use: "nail polish"
- Avoid: "journey"

## Devices
- A fridge calendar after every chapter.

## Chapters
| # | Title | Job | Beats | Words | File |
| --- | --- | --- | --- | --- | --- |
| 1 | Why starters die | Diagnose why the last starter died | hook | 300 | [01](chapters/01.md) |
| 2 | A week that feeds itself | Plan feedings around a working week | calendar | 300 | [02](chapters/02.md) |
| 3 | The first loaf | Bake one loaf and judge its crumb | crumb | 300 | [03](chapters/03.md) |

## Imagery
| Image | Chapter | Job | Art direction | File |
| --- | --- | --- | --- | --- |
| Cover | cover | cover | the jar on a shelf | Open: the owner sends the cover |

## Look
Trim: 5x8in
Body: Georgia, serif
Headings: Georgia, serif
Accent: #8a3b12
`

func bookProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "lfm.json"))
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	c["deliverables"] = map[string]any{"book": map[string]any{"author": "Ada Baker", "path": bookPath}}
	raw, _ = json.MarshalIndent(c, "", "  ")
	write := func(rel, content string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lfm.json", string(raw))
	write(book.ConceptFile, bookConcept)
	text := strings.Repeat("The jar sat on the shelf overnight. It rose by morning when the kitchen stayed warm and quiet. Feed it twice. ", 15)
	for i, title := range []string{"Why starters die", "A week that feeds itself", "The first loaf"} {
		write(book.Dir+"/chapters/0"+string(rune('1'+i))+".md", "# "+title+"\n\n"+text+"\n")
	}
	return dir
}

func TestBookIsPublishedBehindItsPath(t *testing.T) {
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
	// An ordinary compile never starts a browser: the prints wait for a release.
	if _, err := os.Stat(filepath.Join(out, "the-weekend-starter.pdf")); err == nil || !result.Book.Pending {
		t.Fatalf("an ordinary compile printed: pending %v, %v", result.Book.Pending, err)
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
