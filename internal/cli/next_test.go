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
	"github.com/Anonyfox/lfm/internal/skills"
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

// The deploy step asks for the compression the release measured with, when
// leaving it off would cost a phone visible time.
func TestDeployAsksForTheCompressionTheReleaseMeasured(t *testing.T) {
	r := compile.Result{Research: research.Summary{Voices: 3}, Concept: concept.Summary{Present: true}, PageURL: "https://x.test/",
		Release: &release.Result{Audit: &release.AuditResult{Delivery: release.Delivery{TextBytes: 190000, CompressedBytes: 55000, UncompressedMS: 646}}}}
	n := nextStep(r, nil, nil)
	if n.Phase != "deploy" || len(n.Notes) != 1 || !strings.Contains(n.Notes[0], "curl -sI --compressed https://x.test/") || !strings.Contains(n.Notes[0], "646 ms") {
		t.Fatalf("%+v", n)
	}
	r.Release.Audit.Delivery = release.Delivery{TextBytes: 3000, CompressedBytes: 1200, UncompressedMS: 8}
	if n := nextStep(r, nil, nil); len(n.Notes) != 0 {
		t.Fatalf("a note for a page compression barely changes: %+v", n.Notes)
	}
}

func TestOutdatedNotesGroupByFolder(t *testing.T) {
	changes := []skills.Copy{
		{Name: "lfm-design", Path: "/s/.agents/skills/lfm-design/SKILL.md", State: skills.Outdated},
		{Name: "lfm-research", Path: "/s/.agents/skills/lfm-research/SKILL.md", State: skills.Outdated},
		{Name: "lfm-design", Path: "/home/.claude/skills/lfm-design/SKILL.md", State: skills.Outdated},
	}
	got := nextStep(compile.Result{Root: "/s/pages/home"}, nil, changes).Notes
	want := []string{"lfm skills in ../../.agents/skills are outdated; 'lfm skills install /s' updates them.", "lfm skills in ../../../home/.claude/skills are outdated; 'lfm skills install /home' updates them."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}

func TestFictionShopNext(t *testing.T) {
	r := compile.Result{Starter: true, Book: &book.Summary{Kind: "fiction", Sell: "shops"}}
	n := nextStep(r, nil, nil)
	if n.Phase != "book" || n.Skill != "lfm-fiction" || !strings.Contains(n.Do, "write deliverables/book/concept.md: premise, cast, world, arc and voice") {
		t.Fatal(n)
	}
	r.Book.Concept = true
	r.Book.Chapters = []book.Chapter{{N: 1, File: "chapters/one.md", Status: "planned"}}
	if n = nextStep(r, nil, nil); n.Phase != "book" || n.Skill != "lfm-fiction" {
		t.Fatal(n)
	}
	r.Book.Chapters = nil
	r.Book.Complete = true
	if n = nextStep(r, nil, nil); !strings.Contains(n.Do, "deliverables/book/listing.md") || n.Skill != "lfm-fiction" {
		t.Fatal(n)
	}
	r.Book.Listing = true
	if n = nextStep(r, nil, nil); n.Phase != "review" || !strings.Contains(n.Do, "distribution/") {
		t.Fatal(n)
	}
	r.Release = &release.Result{}
	if n = nextStep(r, nil, nil); n.Phase != "deploy" || !strings.Contains(n.Do, "distribution/") {
		t.Fatal(n)
	}
}

func TestShopNextKeepsDeliverableWarnings(t *testing.T) {
	r := compile.Result{Starter: true, Book: &book.Summary{Kind: "fiction", Sell: "shops", Concept: true, Complete: true, Listing: true}, Validation: &check.Report{Warnings: 1, Diagnostics: []check.Diagnostic{{Rule: "page.rule", Severity: diagnostic.Warning}}}}
	if n := nextStep(r, nil, nil); n.Phase != "review" {
		t.Fatal(n)
	}
	r.Validation.Warnings++
	r.Validation.Diagnostics = append(r.Validation.Diagnostics, check.Diagnostic{Rule: "book.machine", Severity: diagnostic.Warning})
	if n := nextStep(r, nil, nil); n.Phase != "fix" || !strings.Contains(n.Do, "deliverable warnings") {
		t.Fatal(n)
	}
}
