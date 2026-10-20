package cli

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/document"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

// Every deliverable has its place in the next step, and one sold only in
// the bundle is listed through the bundle, whichever units it holds.
func TestNextStepCoversEveryDeliverable(t *testing.T) {
	base := compile.Result{Research: research.Summary{Voices: 3}, Concept: concept.Summary{Present: true}, Release: &release.Result{}, PageURL: "https://x.test/"}
	start := map[string]func(*compile.Result){
		"book":        func(r *compile.Result) { r.Book = &book.Summary{} },
		"printable":   func(r *compile.Result) { r.Printable = &printable.Summary{} },
		"spreadsheet": func(r *compile.Result) { r.Spreadsheet = &spreadsheet.Summary{} },
		"document":    func(r *compile.Result) { r.Document = &document.Summary{} },
		"deck":        func(r *compile.Result) { r.Deck = &deck.Summary{} },
	}
	for _, kind := range bundle.Kinds {
		set, ok := start[kind]
		if !ok {
			t.Fatalf("no next step for a new %s", kind)
		}
		r := base
		set(&r)
		if n := nextStep(r, nil, nil); n.Phase != kind || n.Skill != "lfm-"+kind {
			t.Errorf("%s: %+v", kind, n)
		}
	}
	kit := base
	kit.Deck = &deck.Summary{Concept: true, Decks: 1, Sell: "bundle"}
	kit.Document = &document.Summary{Concept: true, Documents: 1, Sell: "bundle"}
	kit.Bundle = &bundle.Summary{Units: []string{"deck", "document"}}
	if n := nextStep(kit, nil, nil); n.Phase != "bundle" || !strings.Contains(n.Do, "deliverables/bundle/listing.md, the shop texts for the deck and document") {
		t.Errorf("bundle listing: %+v", n)
	}
	// bundle.warning is esbuild's, about the page's scripts and styles.
	kit.Bundle.Listing = true
	kit.Validation = &check.Report{Warnings: 1, Diagnostics: []check.Diagnostic{{Rule: "bundle.warning", Severity: diagnostic.Warning}}}
	if n := nextStep(kit, nil, nil); n.Phase != "fix" || strings.Contains(n.Do, "bundle finding") || pageWarnings(kit) != 1 {
		t.Errorf("bundler warning: %+v, %d page warnings", n, pageWarnings(kit))
	}
}
