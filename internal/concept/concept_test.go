package concept

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/research"
)

// fixtureResearch has 30 voices on 4 platforms and 6 peers.
func fixtureResearch(t *testing.T) (*research.Voices, *research.Competitors) {
	t.Helper()
	var v strings.Builder
	v.WriteString("# Human voices: X\n\n## Batch 2026-10-17 · from zero · queries: \"q\" · unreachable: none\n\n")
	platforms := []string{"Reddit", "YouTube", "Forum", "App Store"}
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&v, "### V%03d · %s · buyer · posted 2026-0%d-01 · fetched 2026-10-17 · via fetch\n> voice %d\n<https://example.com/v%d>\n\n", i, platforms[i%4], 1+i%9, i, i)
	}
	voices, problems := research.ParseVoices(v.String())
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	var p strings.Builder
	p.WriteString("# Competitor analysis: X\n\n## Batch 2026-10-17 · from zero\n\n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&p, "### P%03d · Peer %d · https://example.com/p%d · fetched 2026-10-17 · via browser\n", i, i, i)
		for _, f := range research.PeerFields {
			fmt.Fprintf(&p, "- %s: x\n", f)
		}
	}
	peers, _ := research.ParseCompetitors(p.String())
	return &voices, &peers
}

const good = `# Concept: X

## Voices

- They count the days until the next shift V001 V002
- Sleep is negotiated with the household V003 V004
- They photograph their blackout setups V005 V006
- Complaints about apps that assume nine to five V007 V008
- Rituals before the first night V009 V010
- Words they use: "zombie day" V011 V012
- Pride in surviving rotations V013 V014
- Dread of the switch back to days V015 V016

## Buyer

Nurses on rotating nights who already tried an app and quit it V007 V017.

## Decision path

1. Is this for my rota? → show their own week first
2. Will it work after nights? → a worked schedule from the research
3. What do I get? → the printable kit and its price

## Mechanism

Chosen: a rota planner that prints a schedule V018
Rejected: quiz: says little about their actual week
Rejected: explainer: no first value before scrolling

## Offer

Product: the printable rota kit
Price: 19 EUR, once
Delivery: PDF by email within minutes

## Facts

- Fact one V001
- Fact two V002
- Fact three V003
- Fact four P001
- Fact five P002
- Fact six https://example.org/study
- Fact seven V019
- Fact eight V020
- Fact nine V021
- Fact ten P003

## Axes

| Axis | Market | Voices | Chosen |
| --- | --- | --- | --- |
| Tone | clinical P001 P002 | joking V022 | joking |
| Light | bright P003 | dim rooms V023 | dim |
| Proof | ratings P004 | rota photos V024 | rota photos |
| Pace | still P005 | busy V025 | busy |
| Imagery | stock P006 | own photos V026 | own photos |

## Sparks

- The zombie day after the last night V011 V012
- Blackout setups shown off like trophies V005 V006 V026
- Counting down the rota on the fridge V001 V002

## Directions

1. App screenshot hero V001 → struck: the market baseline
2. Calm moon and stars V002 → struck: topic cliché
3. Fridge rota countdown V001 V002 → chosen
  Pushed: the rota flips day by day toward the zombie day V011
4. Trophy wall of blackout rooms V005 → struck: slows the first value
  Pushed: a wall the visitor adds their own room to
5. Zombie day survival log V011 → struck: too narrow
6. Household negotiation script V003 → struck: not the buyer's job
7. Shift clock dial V013 → struck: survives a peer swap
8. Night bus timetable V027 → struck: wrong arrival

## Concept

The page is the fridge rota, counting down to the zombie day V011 V001.

## Spine

1. Rota: the visitor enters their nights and sees their week
2. Plan: the worked schedule and why it works
3. Kit: what the printable contains and its price

## Palette

| Role | Color | Source |
| --- | --- | --- |
| ground | #1b1f2a | dim rooms V023 |
| text | #f2efe6 | fridge paper V001 |
| accent | #e8a33d | sodium night light V028 |

## Type

| Role | Family | Source |
| --- | --- | --- |
| display | Recursive | rota handwriting V029 |
| text | system-ui | legibility on a phone at 4am |

## Form

Paper slips on a dark ground, square corners, no shadows.

## Imagery

| Image | Job | Art direction | Source |
| --- | --- | --- | --- |
| fridge rota | protagonist | phone photo at night V005 | generated |

## Motion

- Moment: the countdown flips to zero
- Feedback: entered nights stamp onto the rota
- Reduced motion: the final state shows at once

## Copy

- Use: "zombie day", “rota” V011 V012
- Joke about the rota, never about patients V022
- Avoid wellness language V030

## Changelog

- 2026-10-18: first concept from the batch of 2026-10-17
`

func check(t *testing.T, text string) Report {
	t.Helper()
	v, p := fixtureResearch(t)
	return Check(text, v, p)
}

func TestGoodConceptIsClean(t *testing.T) {
	r := check(t, good)
	for _, d := range r.Diagnostics {
		t.Errorf("%s line %d: %s", d.Rule, d.Location.Line, d.Message)
	}
	s := r.Summary
	if s.Chosen != "Fridge rota countdown V001 V002" || s.Voices < 20 || s.Platforms != 4 || s.Peers != 6 || s.Colors != 3 || s.Faces != 2 || s.Imagery != 1 {
		t.Fatalf("summary %+v", s)
	}
}

func TestFormatFindings(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"missing section":   {"## Form\n\nPaper slips on a dark ground, square corners, no shadows.\n\n", "", "Missing section Form"},
		"unknown section":   {"## Copy", "## Words", "Unknown section"},
		"order":             {"## Buyer\n\nNurses on rotating nights who already tried an app and quit it V007 V017.\n\n", "", "Missing section Buyer"},
		"no verdict":        {"Night bus timetable V027 → struck: wrong arrival", "Night bus timetable V027", "no verdict"},
		"two chosen":        {"Night bus timetable V027 → struck: wrong arrival", "Night bus timetable V027 → chosen", "marks 2 as chosen"},
		"first chosen":      {"1. App screenshot hero V001 → struck: the market baseline", "1. App screenshot hero V001 → chosen", "first two directions"},
		"spark single":      {"- The zombie day after the last night V011 V012", "- The zombie day after the last night V011", "fewer than two voices"},
		"axis without peer": {"| Light | bright P003 |", "| Light | bright |", "lacks peer IDs"},
		"bad hex":           {"#e8a33d", "orange", "#rrggbb"},
		"motion field":      {"- Feedback: entered nights stamp onto the rota\n", "", "Motion lacks \"Feedback:\""},
		"path arrow":        {"1. Is this for my rota? → show their own week first", "1. Is this for my rota? show their own week first", "lacks \" → \""},
		"placeholder":       {"Paper slips on a dark ground", "<shape, corners …>", "placeholder"},
		"uncited fact":      {"- Fact one V001", "- Fact one", "Fact without a source"},
		"changelog date":    {"- 2026-10-18: first concept", "- first concept", "does not start with a date"},
		"offer field":       {"Delivery: PDF by email within minutes\n", "", "Offer lacks \"Delivery:\""},
		"prose family":      {"| text | system-ui |", "| text | system-ui, then Georgia at small sizes |", "is prose, not a font name"},
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(good, tc.from) {
				t.Fatalf("fixture lacks %q", tc.from)
			}
			r := check(t, strings.Replace(good, tc.from, tc.to, 1))
			if !has(r, "", tc.want) {
				t.Fatalf("no finding containing %q: %v", tc.want, messages(r))
			}
		})
	}
}

func TestCopyPhrases(t *testing.T) {
	r := check(t, good)
	if strings.Join(r.Summary.Use, "|") != "zombie day|rota" {
		t.Fatal(r.Summary.Use)
	}
	if r := check(t, strings.Replace(good, `- Use: "zombie day", “rota” V011 V012`, "- Use: zombie day V011", 1)); !has(r, RuleFormat, "Use names no quoted phrase") {
		t.Fatal(messages(r))
	}
}

func TestOpenQuestions(t *testing.T) {
	r := check(t, strings.Replace(good, "Price: 19 EUR, once", "Price: Open: the owner sets it", 1))
	if !has(r, RuleOpen, "Open question in Offer: the owner sets it") || r.Summary.Open != 1 {
		t.Fatal(messages(r))
	}
	if has(r, RuleFormat, "Offer lacks") {
		t.Fatal("an open field is answered, by a question")
	}
	row := check(t, strings.Replace(good, "| phone photo at night V005 | generated |", "| phone photo at night V005 | Open: the owner photographs it |", 1))
	if !has(row, RuleOpen, `Open question in Imagery row "fridge rota": the owner photographs it`) || row.Summary.Imagery != 0 {
		t.Fatal(messages(row), row.Summary.Imagery)
	}
}

func TestEvidenceFindings(t *testing.T) {
	r := check(t, strings.Replace(good, "V030", "V099", 1))
	if !has(r, RuleEvidence, "V099 does not exist") {
		t.Fatal(messages(r))
	}
	v, p := fixtureResearch(t)
	thin := Check(good, &research.Voices{Entries: v.Entries[:3]}, p)
	if has(thin, RuleEvidence, "cites") && !has(thin, RuleEvidence, "does not exist") {
		t.Fatal("unknown IDs must be reported when the research is smaller")
	}
	if r := Check(good, nil, nil); !has(r, RuleEvidence, "no research") {
		t.Fatal(messages(r))
	}
	narrow := strings.Builder{}
	narrow.WriteString(good[:strings.Index(good, "## Buyer")])
	if r := check(t, narrow.String()+strings.Repeat("", 0)); !has(r, RuleEvidence, "cites") {
		t.Fatal("a concept citing few voices must be reported:", messages(r))
	}
}

func TestDefaultsFindings(t *testing.T) {
	for name, tc := range map[string]struct{ from, to, want string }{
		"stock accent":  {"#e8a33d", "#6366f1", "stock generator accent"},
		"house palette": {"| ground | #1b1f2a | dim rooms V023 |\n| text | #f2efe6 | fridge paper V001 |\n| accent | #e8a33d |", "| ground | #f4f1ea | dim rooms V023 |\n| text | #1b1f2a | fridge paper V001 |\n| accent | #d97757 |", "assistant house palette"},
		"unreadable":    {"| ground | #1b1f2a | dim rooms V023 |\n| text | #f2efe6 | fridge paper V001 |\n| accent | #e8a33d |", "| ground | #555555 | dim rooms V023 |\n| text | #777777 | fridge paper V001 |\n| accent | #666666 |", "No two palette colors reach 4.5:1"},
		"roster face":   {"| display | Recursive |", "| display | Space Grotesk |", "display roster"},
		"stock sans":    {"| display | Recursive |", "| display | Inter |", "Every role uses a stock sans"},
		"imageless":     {"| Image | Job | Art direction | Source |\n| --- | --- | --- | --- |\n| fridge rota | protagonist | phone photo at night V005 | generated |", "None: the page is a printed form V001", "plans no imagery"},
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(good, tc.from) {
				t.Fatalf("fixture lacks %q", tc.from)
			}
			if r := check(t, strings.Replace(good, tc.from, tc.to, 1)); !has(r, RuleDefaults, tc.want) {
				t.Fatalf("no finding containing %q: %v", tc.want, messages(r))
			}
		})
	}
}

func FuzzCheckNeverPanics(f *testing.F) {
	f.Add(good)
	f.Add("# Concept: X\n## Axes\n| a |\n|---|\n- 1. → →")
	f.Fuzz(func(t *testing.T, text string) {
		lines := strings.Count(text, "\n") + 1
		for _, d := range Check(text, nil, nil).Diagnostics {
			if d.Location.Line < 1 || d.Location.Line > lines+1 {
				t.Fatalf("finding at line %d of %d", d.Location.Line, lines)
			}
		}
	})
}

func has(r Report, rule, text string) bool {
	for _, d := range r.Diagnostics {
		if (rule == "" || d.Rule == rule) && strings.Contains(d.Message, text) {
			return true
		}
	}
	return false
}

func messages(r Report) []string {
	var out []string
	for _, d := range r.Diagnostics {
		out = append(out, fmt.Sprintf("%s:%d %s", d.Rule, d.Location.Line, d.Message))
	}
	return out
}
