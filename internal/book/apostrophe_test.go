package book

import (
	"slices"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/check"
)

// slop is a chapter of stock lines whose patterns hold apostrophes.
const slop = `# Why starters die

In this chapter, we'll look at why a starter dies. Let's dive in.

The plan isn't a budget. It's a promise you keep every week, and you've got this.

Don't worry about the smell. It isn't a failure. It's a signal.

## Wrapping up

You're not alone. Let's take a closer look at the jar next time.
`

// TestCurlyApostrophesReadLikeStraightOnes checks a chapter typed with
// straight apostrophes and the same chapter as writers, typographers and
// models type it, with curly ones: the findings must be the same, and the
// stock lines must be found.
func TestCurlyApostrophesReadLikeStraightOnes(t *testing.T) {
	findings := func(chapter, avoid string) []string {
		p, voices, peers := fixture(t)
		writeFile(t, p.Root, ConceptFile, []byte(strings.Replace(testConcept, `- Avoid: "voyage", "game-changer"`, `- Avoid: "voyage", "game-changer", "`+avoid+`"`, 1)))
		writeFile(t, p.Root, Dir+"/chapters/01-why.md", []byte(chapter))
		r := Check(p, testConfig, true, nil, "", voices, peers)
		var out []string
		for _, d := range r.Diagnostics {
			if d.Location.File == Dir+"/chapters/01-why.md" {
				out = append(out, d.Rule+" "+check.Straight(d.Message))
			}
		}
		slices.Sort(out)
		return out
	}
	curly := strings.ReplaceAll(slop, "'", "’")
	straight := findings(slop, "don't worry")
	if got := findings(curly, "don’t worry"); !slices.Equal(got, straight) {
		t.Errorf("curly apostrophes change the findings:\nstraight:\n%s\ncurly:\n%s", strings.Join(straight, "\n"), strings.Join(got, "\n"))
	}
	if got := findings(curly, "don't worry"); !slices.Equal(got, straight) {
		t.Errorf("a straight Avoid phrase misses curly text:\n%s", strings.Join(got, "\n"))
	}
	all := strings.Join(straight, "\n")
	for _, want := range []string{"Let's dive", "we'll", "take a closer look", "don't worry"} {
		if !strings.Contains(strings.ToLower(all), strings.ToLower(want)) {
			t.Errorf("no finding names %q:\n%s", want, all)
		}
	}
}

// The quality measures read either apostrophe once the chapter is parsed.
func TestCurlyApostrophesInTheQualityMeasures(t *testing.T) {
	for _, src := range []string{
		"The plan isn't a budget. It's a promise you keep every week.\n",
		"The plan isn’t a budget. It’s a promise you keep every week.\n",
	} {
		_, p := parseChapter([]byte(src))
		if q := measure(p, "en"); len(q.corrective) != 1 {
			t.Errorf("corrective framing in %q: got %d, want 1", src, len(q.corrective))
		}
	}
	_, p := parseChapter([]byte("Don’t wait for it. It doesn’t work.\n"))
	bl, _ := bookLangOf("en")
	if o := opener(p.blocks[0].text); o != "don't wait" || !negated(p.blocks[0].text, bl) {
		t.Errorf("opener %q, negated %v", o, negated(p.blocks[0].text, bl))
	}
	if !containsPhrase("You’re not behind, you’re early.", "you're early") || !containsPhrase("You're early.", "you’re early") {
		t.Error("phrase matching across apostrophes")
	}
}
