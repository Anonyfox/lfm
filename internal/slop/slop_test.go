package slop

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	for _, r := range Rules() {
		if r.ID == "copy.tells" {
			continue
		} // Its established phrase/threshold matrix lives in check.
		t.Run(r.ID, func(t *testing.T) {
			for _, tc := range []struct {
				text string
				want bool
			}{{r.Example, true}, {r.Quiet, false}} {
				if got := Match(r.ID, r.Language, []string{tc.text}, nil); (len(got) > 0) != tc.want {
					t.Errorf("%q: want hit %v, got %+v", tc.text, tc.want, got)
				}
			}
			if err := ValidateAllow(Allow{Rule: r.ID, Reason: "Verbatim product name"}); err != nil {
				t.Fatal(err)
			}
			if got := Match(r.ID, r.Language, []string{r.Example}, []Allow{{Rule: r.ID, Reason: "Licensed quotation"}}); len(got) != 0 {
				t.Fatalf("allowed rule: %+v", got)
			}
		})
	}
}

func TestAllow(t *testing.T) {
	for _, tc := range []struct {
		a     Allow
		valid bool
	}{
		{Allow{Rule: "copy.superlative", Phrase: "Traumhaft Hotel", Reason: "Registered name"}, true},
		{Allow{Rule: "copy.superlative", Reason: "  "}, false},
		{Allow{Rule: "typo", Reason: "Registered name"}, false},
	} {
		if err := ValidateAllow(tc.a); (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc.a, err)
		}
	}
	a := []Allow{{Rule: "copy.superlative", Phrase: "Traumhaft Hotel", Reason: "Registered name"}}
	for _, tc := range []struct {
		text  string
		count int
	}{{"Traumhaft Hotel", 0}, {"Traumhaft Hotel mit traumhafter Aussicht.", 1}, {"NichtTraumhaft Hotel", 0}} {
		if got := Match("copy.superlative", "de", []string{tc.text}, a); len(got) != tc.count {
			t.Errorf("%q: %+v", tc.text, got)
		}
	}
}

func TestLanguage(t *testing.T) {
	for _, tc := range []struct{ tag, text, want string }{{"de-DE", "the house", "de"}, {"en_GB", "das Haus", "en"}, {"", "Das Haus ist für die Gäste.", "de"}, {"", "The house is for you.", "en"}, {"", "1234", "any"}, {"fr", "das Haus", "fr"}} {
		if got := Language(tc.tag, tc.text); got != tc.want {
			t.Errorf("%+v: %s", tc, got)
		}
	}
}

func FuzzMatch(f *testing.F) {
	f.Add("Nicht nur schön, sondern praktisch. 🚀")
	f.Add("Zimmer 13—14. Juwelier Kaufmann.")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 32768 {
			t.Skip()
		}
		for _, r := range Rules() {
			for _, hit := range Match(r.ID, Language("", s), strings.Split(s, "\n"), nil) {
				if hit.Block < 0 || hit.Block >= len(strings.Split(s, "\n")) {
					t.Fatal(hit)
				}
			}
		}
	})
}

func TestSentenceRulesRequireProse(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		blocks   []string
		want     int
	}{
		{"repeated prose", "copy.duplicate-sentence", []string{"The workbook includes 6 exercises for planning each week with your team.", "The workbook includes 6 exercises for planning each week with your team."}, 1},
		{"different quantities", "copy.duplicate-sentence", []string{"The workbook includes 6 exercises for planning each week with your team.", "The workbook includes 9 exercises for planning each week with your team."}, 0},
		{"citation labels", "copy.duplicate-sentence", []string{"Bundesministerium der Justiz und für Verbraucherschutz / Bundesamt für Justiz: JArbSchG § 8", "Bundesministerium der Justiz und für Verbraucherschutz / Bundesamt für Justiz: JArbSchG § 8"}, 0},
		{"data rows", "copy.duplicate-sentence", []string{"The workbook includes six exercises · for planning each week with your team.", "The workbook includes six exercises · for planning each week with your team."}, 0},
		{"label rhythm", "copy.rhythm", []string{strings.Repeat("Source label with five words\n", 6)}, 0},
		{"interrupted rhythm", "copy.rhythm", []string{"We plan the next week. You write the first task. They make the next choice.", "Source label", "We check the final list. You start the next step. They read the last page."}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.id, "en", tc.blocks, nil); len(got) != tc.want {
				t.Fatalf("want %d, got %+v", tc.want, got)
			}
		})
	}
}
