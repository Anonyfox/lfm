package aitext

import (
	"math"
	"strings"
	"testing"
)

func TestPassagesContributions(t *testing.T) {
	text := fixtures(t, "generated")["opus-01.md"]
	paragraphs := Paragraphs(text)
	before := Analyze(paragraphs, "en")
	ranked := Passages(paragraphs, "en", 3)
	if len(ranked) == 0 || len(ranked) > 3 {
		t.Fatalf("%+v", ranked)
	}
	first := 0
	for _, window := range windows(paragraphs) {
		baseline := analyzeWindow(window, "en")
		for _, passage := range ranked {
			i := passage.Paragraph - first
			if i < 0 || i >= len(window) {
				continue
			}
			rest := append(append([]string{}, window[:i]...), window[i+1:]...)
			without := analyzeWindow(rest, "en")
			expected := 0.0
			if without.scored {
				expected = baseline.Score - without.Score
			}
			if math.Abs(passage.Contribution-expected) > 1e-12 {
				t.Fatalf("%+v want %f", passage, expected)
			}
		}
		first += len(window)
	}
	for i := 1; i < len(ranked); i++ {
		if ranked[i].Contribution > ranked[i-1].Contribution {
			t.Fatal("not ranked")
		}
	}
	after := Analyze(paragraphs, "en")
	if before.Score != after.Score || before.Likely != after.Likely {
		t.Fatal("reporting changed estimate")
	}
	if Passages(paragraphs, "en", 0) != nil || len(Passages([]string{"short"}, "en", 3)) != 0 {
		t.Fatal("unexpected short results")
	}
}

func FuzzPassages(f *testing.F) {
	f.Add("A short paragraph.\n\nAnother paragraph.", "en")
	f.Fuzz(func(t *testing.T, text, lang string) {
		if len(text) > 4096 || len(lang) > 32 {
			t.Skip()
		}
		paragraphs := strings.Split(text, "\n\n")
		if len(paragraphs) > 32 {
			t.Skip()
		}
		ranked := Passages(paragraphs, lang, 3)
		if len(ranked) > 3 {
			t.Fatal("uncapped")
		}
		for _, passage := range ranked {
			if passage.Paragraph < 0 || passage.Paragraph >= len(paragraphs) || math.IsNaN(passage.Contribution) {
				t.Fatalf("%+v", passage)
			}
		}
	})
}

func TestPassageAttributionNeedsRemainingProse(t *testing.T) {
	paragraphs := []string{strings.Repeat("The garden needs water every morning. ", 30)}
	passages := Passages(paragraphs, "en", 3)
	if len(passages) != 1 || passages[0].Attributed || passages[0].Paragraph != 0 || passages[0].Contribution != 0 {
		t.Fatalf("%+v", passages)
	}
}
