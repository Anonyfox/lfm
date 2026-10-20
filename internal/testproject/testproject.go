// Package testproject builds the small projects lfm's tests compile: one per
// deliverable, and bundles of them, each in a fresh temp directory. Only
// tests import it.
package testproject

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/scaffold"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

// The unlisted download paths of the deliverables.
const (
	BookPath      = "q7w2k9m4x8c3v6b1n5zt"
	PrintablePath = "p7m2x9q4w8e3r6t5y1u0"
	SheetPath     = "x7m2x9q4w8e3r6t5y1u0"
	DocPath       = "k3n8q2w7x5r9t4m6p1"
	DeckPath      = "q7w2e9r4t1y8u3i6o5"
	BundlePath    = "b7m2x9q4w8e3r6t5y1u0"
)

// Internal is the repository's internal/ directory, where the deliverables'
// testdata live, wherever the test runs from.
func Internal() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

// Example is the example project in the repository's testdata.
func Example() string { return filepath.Join(filepath.Dir(Internal()), "testdata", "example") }

// Configure edits lfm.json.
func Configure(t testing.TB, dir string, edit func(map[string]any)) {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(dir, "lfm.json"))
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	edit(c)
	raw, _ = json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "lfm.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// CopyTree copies the files under src to dst.
func CopyTree(src, dst string) {
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		os.MkdirAll(filepath.Join(dst, filepath.Dir(rel)), 0o755)
		data, _ := os.ReadFile(path)
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

func scaffolded(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// BookConcept plans a three-chapter book.
const BookConcept = `# Book: Starter rescue

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

// Book is a project selling a written three-chapter book, without a listing.
func Book(t testing.TB) string {
	t.Helper()
	dir := scaffolded(t)
	raw, _ := os.ReadFile(filepath.Join(dir, "lfm.json"))
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	c["deliverables"] = map[string]any{"book": map[string]any{"author": "Ada Baker", "path": BookPath}}
	raw, _ = json.MarshalIndent(c, "", "  ")
	write := func(rel, content string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lfm.json", string(raw))
	write(book.ConceptFile, BookConcept)
	text := strings.Repeat("The jar sat on the shelf overnight. It rose by morning when the kitchen stayed warm and quiet. Feed it twice. ", 15)
	for i, title := range []string{"Why starters die", "A week that feeds itself", "The first loaf"} {
		write(book.Dir+"/chapters/0"+string(rune('1'+i))+".md", "# "+title+"\n\n"+text+"\n")
	}
	return dir
}

// Printable is a project selling a two-maze pack in A4 and Letter.
func Printable(t testing.TB) string {
	t.Helper()
	dir := scaffolded(t)
	Configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"printable": map[string]any{"path": PrintablePath, "version": "2027.1"}}
	})
	WritePrintable(t, dir)
	return dir
}

// WritePrintable adds a small maze pack in deliverables/printable.
func WritePrintable(t testing.TB, dir string) {
	t.Helper()
	files := map[string]string{
		printable.ConceptFile: "# Printable: Maze Pack\n\n## Buyer\nParents with a rainy afternoon.\n- Not for: toddlers.\n\n## Promise\nTwo mazes and their solutions, printed in a minute.\n\n" +
			"## Contents\n| Part | Pages | Job |\n| --- | --- | --- |\n| Cover | 1 | shows the pack and what is inside |\n| Guide | 1 | explains printing at actual size |\n| Mazes | 2 | two mazes to solve with a pencil |\n| Key | 1 | both solutions to check |\n\n" +
			"## Editions\n- Sizes: a4, letter\n\n## Look\nBody: system-ui, sans-serif\nHeadings: system-ui, sans-serif\nAccent: #2b6e5a\n",
		printable.TemplateFile: `{{- $a := maze 10 12 1 -}}{{- $b := maze 12 14 2 -}}
<section class="page" data-part="Cover" data-role="cover"><h1>{{.Name}}</h1></section>
<section class="page" data-part="Guide" data-role="guide"><p>Print at 100%. Solutions on page <span class="page-ref" data-ref="key"></span>.</p></section>
<section class="page" data-part="Mazes" data-role="puzzle">{{$a.SVG 10 false}}</section>
<section class="page" data-part="Mazes" data-role="puzzle">{{$b.SVG 10 false}}</section>
<section class="page" data-part="Key" data-role="key" id="key">{{$a.SVG 6 true}}{{$b.SVG 6 true}}</section>
`,
		printable.Dir + "/printable.css": ".maze { width: 140mm; display: block; margin: 0 auto; }\n.page[data-role=key] .maze { width: 80mm; }\n",
		printable.ListingFile:            "# Listing: Maze Pack\n\n## Title\nPrintable Maze Pack for Kids, A4 and Letter\n\n## Description\n" + strings.Repeat("Two calm mazes for a rainy afternoon, with solutions. ", 5) + "\n\n## Tags\n- maze printable\n\n## Price\n3.00 EUR\n\n## AI\nText: none\nImages: none\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Spreadsheet is a project selling a paycheck budget workbook.
func Spreadsheet(t testing.TB) string {
	t.Helper()
	dir := scaffolded(t)
	Configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"spreadsheet": map[string]any{"path": SheetPath, "version": "2027.1",
			"sheets": map[string]any{"en": "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789/edit"}}}
		c["seller"] = map[string]any{"name": "Calm Money"}
	})
	src := filepath.Join(Internal(), "spreadsheet", "testdata", "budget")
	dst := filepath.Join(dir, filepath.FromSlash(spreadsheet.Dir))
	os.MkdirAll(dst, 0o755)
	for _, name := range []string{"concept.md", "workbook.json"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dst, name), data, 0o644)
	}
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Paycheck Budget\n\n## Title\nPaycheck Budget Spreadsheet for Google Sheets and Excel\n\n## Description\n"+strings.Repeat("Know what is safe to spend until payday, every evening, in one simple list. ", 4)+"\n\n## Tags\n- budget spreadsheet\n\n## Price\n7.90 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts edited with Claude; formulas computed and checked by software.\n"), 0o644)
	return dir
}

// Document is a project selling an allotment society's document kit.
func Document(t testing.TB) string {
	t.Helper()
	dir := scaffolded(t)
	Configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"document": map[string]any{"path": DocPath, "version": "2027.1"}}
		c["seller"] = map[string]any{"name": "Riverside Press"}
	})
	dst := filepath.Join(dir, filepath.FromSlash(document.Dir))
	CopyTree(filepath.Join(Internal(), "document", "testdata", "club"), dst)
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Allotment Society Kit\n\n## Title\nAllotment Society Templates for Word and Google Docs, Minutes, Invoice, Fair Program\n\n## Description\n"+strings.Repeat("Run your community garden's paperwork in one evening: minutes, plot fee invoices and the harvest fair program, ready to fill. ", 3)+"\n\n## Tags\n- allotment templates\n\n## Price\n9.90 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts edited with Claude; layout and checks by software.\n"), 0o644)
	return dir
}

// Deck is a project selling a fire safety briefing in English and German.
func Deck(t testing.TB) string {
	t.Helper()
	dir := scaffolded(t)
	Configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"deck": map[string]any{"path": DeckPath, "version": "2027.1"}}
		c["seller"] = map[string]any{"name": "Riverside Press"}
	})
	dst := filepath.Join(dir, filepath.FromSlash(deck.Dir))
	for src, under := range map[string]string{filepath.Join(Internal(), "deck", "testdata", "briefing"): "", filepath.Join(Internal(), "document", "testdata", "club", "fonts"): "fonts"} {
		CopyTree(src, filepath.Join(dst, under))
	}
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Fire Safety Briefing\n\n## Title\nFire Safety Briefing for Bakeries, Editable PowerPoint and Google Slides Training with Quiz\n\n## Description\n"+strings.Repeat("Brief your staff on fire safety tonight: the slides, the speaker notes and a quiz with its answer key, in English and German. ", 3)+"\n\n## Tags\n- fire safety training\n\n## Price\n19.00 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts drafted with Claude and reviewed; layout and checks by software.\n"), 0o644)
	return dir
}

// BookPrintableBundle is the book and the maze pack sold together as one
// bundle, the printable only in it.
func BookPrintableBundle(t testing.TB) string {
	t.Helper()
	dir := Book(t)
	WritePrintable(t, dir)
	Configure(t, dir, func(c map[string]any) {
		d := c["deliverables"].(map[string]any)
		d["printable"] = map[string]any{"sell": "bundle"}
		d["bundle"] = map[string]any{"name": "Starter Kit", "path": BundlePath}
	})
	os.MkdirAll(filepath.Join(dir, "deliverables", "bundle"), 0o755)
	os.WriteFile(filepath.Join(dir, "deliverables", "bundle", "listing.md"), []byte("# Listing: Starter Kit\n\n## Title\nSourdough Starter Kit: ebook and maze pack\n\n## Description\n"+strings.Repeat("The book and the pack for a rainy weekend with a starter. ", 5)+"\n\n## Tags\n- sourdough kit\n\n## Price\n9.00 EUR\n\n## AI\nText: none\nImages: none\n"), 0o644)
	return dir
}

// DeckDocumentBundle is the deck with the club's documents as its handouts,
// both sold only in one bundle.
func DeckDocumentBundle(t testing.TB) string {
	t.Helper()
	dir := Deck(t)
	CopyTree(filepath.Join(Internal(), "document", "testdata", "club"), filepath.Join(dir, "deliverables", "document"))
	Configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{
			"deck":     map[string]any{"sell": "bundle", "version": "2027.1"},
			"document": map[string]any{"sell": "bundle"},
			"bundle":   map[string]any{"name": "Fire Safety Kit", "path": BundlePath, "units": []string{"deck", "document"}},
		}
	})
	os.MkdirAll(filepath.Join(dir, "deliverables", "bundle"), 0o755)
	os.Rename(filepath.Join(dir, "deliverables", "deck", "listing.md"), filepath.Join(dir, "deliverables", "bundle", "listing.md"))
	return dir
}
