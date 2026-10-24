package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/golden"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
	"github.com/Anonyfox/lfm/internal/term"
)

func TestReportHelpers(t *testing.T) {
	t.Parallel()
	n := func(v int) *int { return &v }
	for _, tc := range []struct{ got, want string }{
		{score(nil), "–"},
		{score(n(0)), "0"},
		{score(n(97)), "97"},
		{metric(""), "–"},
		{metric("1.2 s"), "1.2 s"},
		{sellNote(""), ""},
		{sellNote("page"), ""},
		{sellNote("shops"), " · sold in shops"},
		{sellNote("bundle"), " · sold in the bundle"},
		{groupDigits(0), "0"},
		{groupDigits(999), "999"},
		{groupDigits(1000), "1,000"},
		{groupDigits(45210), "45,210"},
		{groupDigits(1234567), "1,234,567"},
		{groupDigits(-1234), "-1,234"},
		{groupDigits(-100), "-100"},
		{lowerFirst("Use HTTP/2"), "use HTTP/2"},
		{lowerFirst("Électricité"), "électricité"},
		{lowerFirst("x"), "x"},
		{listed(nil), "no deliverables"},
		{listed([]string{"deck"}), "deck"},
		{listed([]string{"deck", "document"}), "deck and document"},
		{listed([]string{"book", "deck", "printable"}), "book, deck and printable"},
		{directionName(""), ""},
		{directionName("Warm kitchen: flour on dark wood V012"), "Warm kitchen"},
		{directionName("Ledger — ruled paper, ink blue"), "Ledger"},
		{directionName("Ledger – ruled paper"), "Ledger"},
		{directionName("Night shift - neon on black"), "Night shift"},
		{directionName("Plain paper V001 P002"), "Plain paper"},
		{directionName(": no head"), ": no head"},
		{directionName("Ein sehr langer Name für eine Richtung, die niemand so kurz fassen wollte"), "Ein sehr langer Name für eine Richtung, die niemand so kurz…"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

// TestReportStates renders the states a real compile of the golden
// projects does not reach: deliverables not started or half done, released
// files, research, a concept, a release and a failed compile.
func TestReportStates(t *testing.T) {
	t.Parallel()
	var w bytes.Buffer
	s := term.Style{}
	section := func(name string) { w.WriteString("## " + name + "\n") }
	n := func(v int) *int { return &v }

	section("book")
	renderBook(&w, s, nil, nil)
	renderBook(&w, s, &book.Summary{}, nil)
	renderBook(&w, s, &book.Summary{Concept: true, Chapters: make([]book.Chapter, 12), Written: 1, Words: 2400, Target: 36000, URL: "https://x.test/p/"}, nil)
	renderBook(&w, s, &book.Summary{Concept: true, Title: "Rye", Chapters: make([]book.Chapter, 2), Words: 0, Target: 1200, Sell: "shops"}, nil)
	renderBook(&w, s, &book.Summary{Concept: true, Title: "Rye", Chapters: make([]book.Chapter, 2), Written: 2, Words: 1250, Target: 1200, Complete: true,
		Files: []string{"rye.epub", "rye.pdf"}, Layout: "fits", Channels: []string{"kdp"}, Sell: "bundle"},
		[]compile.File{{Name: "b1/rye.epub", Bytes: 52000}, {Name: "b1/rye.pdf", Bytes: 1 << 20}, {Name: "b2/rye.epub", Bytes: 52000}})

	section("printable")
	renderPrintable(&w, s, nil)
	renderPrintable(&w, s, &printable.Summary{})
	renderPrintable(&w, s, &printable.Summary{Concept: true, Name: "Mazes"})
	renderPrintable(&w, s, &printable.Summary{Concept: true, Template: true, Name: "Mazes", Editions: []string{"A4"}, Pages: 1, Links: 1, Sell: "shops",
		Files: []string{"mazes-a4.pdf"}, Layout: "fits", Channels: []string{"etsy"}, Images: 1})

	section("spreadsheet")
	renderSpreadsheet(&w, s, nil)
	renderSpreadsheet(&w, s, &spreadsheet.Summary{})
	renderSpreadsheet(&w, s, &spreadsheet.Summary{Concept: true, Name: "Budget"})
	renderSpreadsheet(&w, s, &spreadsheet.Summary{Concept: true, Workbook: true, Name: "Budget", Editions: []string{"en"}, Sheets: 1, Formulas: 1, Checked: []string{"example"},
		Files: []string{"budget.xlsx"}, Compat: "recalculated in LibreOffice", Linked: 1, Channels: []string{"etsy"}, Images: 3, Sell: "bundle"})

	section("document")
	renderDocument(&w, s, nil)
	renderDocument(&w, s, &document.Summary{})
	renderDocument(&w, s, &document.Summary{Concept: true, Name: "Club"})
	renderDocument(&w, s, &document.Summary{Concept: true, Name: "Club", Documents: 1, Fonts: []string{"Plain Serif"}, Editions: []string{"en-a4"}, Fields: 1,
		Files: []string{"club.zip"}, Layout: "1 document prints as planned", Linked: 2, Channels: []string{"etsy"}, Images: 2, Sell: "shops"})

	section("deck")
	renderDeck(&w, s, nil)
	renderDeck(&w, s, &deck.Summary{})
	renderDeck(&w, s, &deck.Summary{Concept: true, Name: "Briefing"})
	renderDeck(&w, s, &deck.Summary{Concept: true, Name: "Briefing", Decks: 2, Slides: 1, Minutes: 1, Quizzes: 1, Fonts: []string{"Text Face", "Heading Face"}, Editions: []string{"en"},
		Files: []string{"briefing.zip"}, Layout: "2 decks print as planned", Linked: 1, Channels: []string{"etsy"}, Images: 1})

	section("bundle")
	renderBundle(&w, s, nil)
	renderBundle(&w, s, &bundle.Summary{Name: "Kit", Sell: "shops"})
	renderBundle(&w, s, &bundle.Summary{Name: "Kit", Units: []string{"book", "printable", "deck"}, Ready: true, Files: []string{"a.pdf"}, URL: "https://x.test/k/", Channels: []string{"etsy"}, Images: 1})

	section("research")
	renderResearch(&w, s, research.Summary{})
	renderResearch(&w, s, research.Summary{Peers: 2})
	renderResearch(&w, s, research.Summary{Voices: 40, Peers: 1, Platforms: []research.Count{{Name: "reddit", N: 20}, {Name: "amazon", N: 10}, {Name: "forum", N: 6}, {Name: "blog", N: 4}},
		Dated: 30, Recent: 12, Oldest: time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC), Newest: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Latest: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)})
	renderResearch(&w, s, research.Summary{Voices: 1, Platforms: []research.Count{{Name: "reddit", N: 1}}, Dated: 1, Recent: 1,
		Oldest: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Newest: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)})

	section("concept")
	renderConcept(&w, s, concept.Summary{})
	renderConcept(&w, s, concept.Summary{Present: true})
	renderConcept(&w, s, concept.Summary{Present: true, Chosen: "Warm kitchen: flour on dark wood V012", Open: 1, Voices: 1, Platforms: 1, Peers: 1, Colors: 1, Faces: 1, Imagery: 1})

	section("release")
	renderRelease(&w, s, nil)
	renderRelease(&w, s, &release.Result{Reason: "no Chrome found"})
	renderRelease(&w, s, &release.Result{Applied: true, CriticalBytes: 1800, OriginalBytes: 9000, LazyImages: 1, PriorityImage: "assets/images/hero.webp"})
	renderRelease(&w, s, &release.Result{Applied: true, CriticalBytes: 1800, OriginalBytes: 9000, LazyImages: 3, Audit: &release.AuditResult{
		Runtime: &release.RuntimeResult{Status: "ok", Chrome: "Chrome 141", Problems: []string{"TypeError"}, DurationMS: 812,
			Layout: &release.LayoutResult{Frames: ".lfm/frames", Count: 12, Previous: true, Changed: []string{"phone-01", "desktop-03"}}},
		Lighthouse: &release.LighthouseResult{Status: "ok", Report: ".lfm/lighthouse.html",
			Scores:   map[string]*int{"performance": n(98), "accessibility": n(100), "best-practices": nil, "seo": n(92)},
			Metrics:  map[string]string{"largest-contentful-paint": "0.9 s", "total-blocking-time": "0 ms", "cumulative-layout-shift": "0"},
			Findings: []release.AuditFinding{{ID: "a"}},
			Host:     []release.AuditFinding{{Title: "Use HTTP/2"}, {Title: ""}, {Title: "Énable text compression"}}},
	}})
	renderRelease(&w, s, &release.Result{Audit: &release.AuditResult{
		Runtime:    &release.RuntimeResult{Status: "ok", Chrome: "Chrome 141", Layout: &release.LayoutResult{Frames: ".lfm/frames", Count: 1, Previous: true}},
		Lighthouse: &release.LighthouseResult{Status: "ok", Host: []release.AuditFinding{{Title: ""}}},
	}})
	renderRelease(&w, s, &release.Result{Audit: &release.AuditResult{
		Runtime: &release.RuntimeResult{Status: "ok", Chrome: "Chrome 141", Layout: &release.LayoutResult{Count: 40, Previous: true,
			Changed: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}}},
	}})
	renderRelease(&w, s, &release.Result{Audit: &release.AuditResult{
		Runtime:    &release.RuntimeResult{Status: "failed", Reason: "Chrome crashed", Layout: &release.LayoutResult{Count: 2}},
		Lighthouse: &release.LighthouseResult{Status: "missing"},
	}})

	section("result")
	renderResult(&w, s, compile.Result{Root: "/x/shop"}, errors.New("build failed"))
	renderResult(&w, s, compile.Result{Root: "/x/shop", Optional: []string{"lighthouse: npm i -g lighthouse"},
		Files:    []compile.File{{Name: "index.html", Bytes: 900}, {Name: "a.css", Bytes: 1}, {Name: "b.js", Bytes: 2}, {Name: "c.png", Bytes: 3}, {Name: "d.png", Bytes: 4}},
		External: []string{"https://cdn.test/x.js"}}, nil)

	golden.Text(t, "report-states.txt", w.String())
}

func TestResearchShortfallIsVisible(t *testing.T) {
	var out bytes.Buffer
	renderResearch(&out, term.Style{}, research.Summary{Shortfalls: []string{"few public sources"}})
	if !strings.Contains(out.String(), "shortfall: few public sources") {
		t.Fatal(out.String())
	}
}
