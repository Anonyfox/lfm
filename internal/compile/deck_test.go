package compile

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
)

const deckPath = "q7w2e9r4t1y8u3i6o5"

func deckProject(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"deck": map[string]any{"path": deckPath, "version": "2027.1"}}
		c["seller"] = map[string]any{"name": "Riverside Press"}
	})
	dst := filepath.Join(dir, filepath.FromSlash(deck.Dir))
	for src, under := range map[string]string{filepath.Join("..", "deck", "testdata", "briefing"): "", filepath.Join("..", "document", "testdata", "club", "fonts"): "fonts"} {
		filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(src, path)
			os.MkdirAll(filepath.Join(dst, under, filepath.Dir(rel)), 0o755)
			data, _ := os.ReadFile(path)
			return os.WriteFile(filepath.Join(dst, under, rel), data, 0o644)
		})
	}
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Fire Safety Briefing\n\n## Title\nFire Safety Briefing for Bakeries, Editable PowerPoint and Google Slides Training with Quiz\n\n## Description\n"+strings.Repeat("Brief your staff on fire safety tonight: the slides, the speaker notes and a quiz with its answer key, in English and German. ", 3)+"\n\n## Tags\n- fire safety training\n\n## Price\n19.00 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts drafted with Claude and reviewed; layout and checks by software.\n"), 0o644)
	return dir
}

func TestDecksAreBuiltCheckedAndDelivered(t *testing.T) {
	dir := deckProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil || result.Deck == nil || result.Deck.Decks != 1 || result.Deck.Slides != 11 || len(result.Deck.Files) != 2 {
		t.Fatalf("%v %+v %v", err, result.Deck, findings(result, "deck."))
	}
	if f := findings(result, "deck."); len(f) > 0 {
		t.Fatalf("findings: %v", f)
	}
	out := filepath.Join(dir, "output", deckPath)
	first, _ := os.ReadFile(filepath.Join(out, "fire-safety-briefing-de.zip"))
	if _, err := Compile(context.Background(), dir, Options{Publish: true}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(out, "fire-safety-briefing-de.zip"))
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("an unchanged set rebuilds different bytes")
	}
	if _, ok := result.Previews[deckPath+"/preview/en.html"]; !ok {
		t.Fatal("no preview")
	}
	if _, err := os.Stat(filepath.Join(dir, "distribution", "deck", "README.md")); err != nil {
		t.Fatalf("the shop package is gone: %v", err)
	}
	if testing.Short() {
		return
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || result.Deck.Pending || result.Deck.Images < 4 || !strings.HasPrefix(result.Deck.Layout, "2 decks print as planned") {
		t.Fatalf("release: %v %+v %v", err, result.Deck, findings(result, "deck."))
	}
	z, err := zip.OpenReader(filepath.Join(out, "fire-safety-briefing-en.zip"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	z.Close()
	for _, want := range []string{"Fire Safety Briefing.pptx", "PDF/Fire Safety Briefing.pdf", "PDF/Fire Safety Briefing (speaker notes, A4).pdf", "PDF/Fire Safety Briefing (speaker notes, US Letter).pdf", "PDF/Fire Safety Briefing (quiz, A4).pdf", "Start here.pdf", "Fonts/Lora-Regular.ttf", "Fonts/OFL.txt"} {
		if !strings.Contains(strings.Join(names, "\n"), want) {
			t.Errorf("the en ZIP lacks %s: %v", want, names)
		}
	}
}

func TestBundleSellsDeckAndDocumentsTogether(t *testing.T) {
	dir := deckProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	// The kit's handouts: the club's documents stand in for them.
	src := filepath.Join("..", "document", "testdata", "club")
	dst := filepath.Join(dir, "deliverables", "document")
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		os.MkdirAll(filepath.Join(dst, filepath.Dir(rel)), 0o755)
		data, _ := os.ReadFile(path)
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{
			"deck":     map[string]any{"sell": "bundle", "version": "2027.1"},
			"document": map[string]any{"sell": "bundle"},
			"bundle":   map[string]any{"name": "Fire Safety Kit", "path": bundlePath, "units": []string{"deck", "document"}},
		}
	})
	os.MkdirAll(filepath.Join(dir, "deliverables", "bundle"), 0o755)
	os.Rename(filepath.Join(dir, "deliverables", "deck", "listing.md"), filepath.Join(dir, "deliverables", "bundle", "listing.md"))
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v %v", err, findings(result, ""))
	}
	b := result.Bundle
	if b == nil || strings.Join(b.Units, " ") != "deck document" || !b.Ready || len(b.Files) != 4 || result.Deck.Sell != "bundle" || result.Document.Sell != "bundle" {
		t.Fatalf("bundle %+v", b)
	}
	for _, want := range []string{"fire-safety-briefing-en.zip", "allotment-society-kit-en-a4.zip"} {
		if _, err := os.Stat(filepath.Join(dir, "output", bundlePath, want)); err != nil {
			t.Errorf("bundle download lacks %s", want)
		}
	}
	// Units sold only in the bundle get no shop packages of their own.
	for _, own := range []string{"deck", "document"} {
		if _, err := os.Stat(filepath.Join(dir, "distribution", own)); err == nil {
			t.Errorf("distribution/%s exists for a unit sold only in the bundle", own)
		}
	}
	if _, browserErr := release.BrowserPath(); browserErr != nil || testing.Short() {
		return
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || result.Bundle.Images < 4 || len(result.Bundle.Channels) == 0 {
		t.Fatalf("release: %v %+v %v", err, result.Bundle, findings(result, "bundle."))
	}
	images, _ := os.ReadDir(filepath.Join(dir, "distribution", "bundle", "images"))
	var names []string
	for _, e := range images {
		names = append(names, e.Name())
	}
	if len(names) < 4 || names[0] != "01-bundle.jpg" || !strings.HasPrefix(names[1], "02-deck-") || !strings.HasPrefix(names[2], "03-document-") {
		t.Fatalf("bundle images %v", names)
	}
	guide, _ := os.ReadFile(filepath.Join(dir, "distribution", "bundle", "README.md"))
	if !bytes.Contains(guide, []byte("Fire Safety Briefing (11 slides")) || !bytes.Contains(guide, []byte("Allotment Society Kit:")) {
		t.Errorf("bundle README lacks the units' facts:\n%s", guide)
	}
}
