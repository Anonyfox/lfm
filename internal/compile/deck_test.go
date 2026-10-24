package compile

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/testproject"
)

func TestDecksAreBuiltCheckedAndDelivered(t *testing.T) {
	t.Parallel()
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
		t.Fatalf("release: %v %+v %v", err, result.Deck, findings(result, ""))
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
	t.Parallel()
	// The kit's handouts: the club's documents stand in for them.
	dir := testproject.DeckDocumentBundle(t)
	t.Cleanup(func() { strictArchives(t, dir) })
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
