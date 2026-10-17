package research

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

const goodVoices = `# Human voices: Night shift sleep

## Batch 2026-10-17 · from zero · queries: "night shift sleep", "can't sleep after nights" · unreachable: Instagram (login wall)

| Metric | Value | Source |
| --- | --- | --- |
| members | 120k | https://www.reddit.com/r/nightshift/ |

### V001 · Reddit (r/nightshift) · buyer · posted 2026-03-02 · fetched 2026-10-17 · via fetch
> Blackout curtains changed my life.
>
> ### This line is part of the quote · not a heading
<https://www.reddit.com/r/nightshift/comments/abc/>

### V002 · TikTok · creator · posted unknown · fetched 2026-10-18 · via browser
> POV: you're asleep at 3pm and the doorbell rings
<https://www.tiktok.com/@nurse/video/123>
Observer note: selfie video in a dark bedroom, caption box at the top.
`

const goodCompetitors = `# Competitor analysis: Night shift sleep

## Batch 2026-10-17 · from zero

### P001 · Sleep app · https://example.com/sleep · fetched 2026-10-17 · via browser
- Opening: "Sleep better on nights"; phone mockup; "Download"
- Palette: navy, white
- Type: one sans
- Imagery: app screenshots
- Motion: none
- Mechanism: app with schedule
- Proof: store rating
- Price: subscription
- Tone: calm
- Takeaway: none

### Grid after this batch

| Column | Value | Peers |
| --- | --- | --- |
| Opening | promise plus app mockup | 1 |
| Palette | navy | 1 |
| Type | one sans | 1 |
| Imagery | screenshots | 1 |
| Motion | none | 1 |
| Mechanism | app | 1 |
| Proof | rating | 1 |
| Price | subscription | 1 |
| Tone | calm | 1 |
| Takeaway | none | 1 |

### Baseline

Navy app pages that promise better sleep.

### Gaps

- printable tools
`

func TestParseValidFiles(t *testing.T) {
	v, problems := ParseVoices(goodVoices)
	if len(problems) != 0 {
		t.Fatalf("%+v", problems)
	}
	if len(v.Batches) != 1 || len(v.Entries) != 2 || v.Entries[0].Platform != "Reddit" || v.Entries[0].Place != "r/nightshift" {
		t.Fatalf("%+v", v)
	}
	if v.Entries[0].Quote != "Blackout curtains changed my life.\n\n### This line is part of the quote · not a heading" {
		t.Fatalf("quote: %q", v.Entries[0].Quote)
	}
	if !v.Entries[1].Posted.IsZero() || v.Entries[1].Observer == "" || v.Batches[0].Unreachable != "Instagram (login wall)" {
		t.Fatalf("%+v", v.Entries[1])
	}
	c, problems := ParseCompetitors(goodCompetitors)
	if len(problems) != 0 {
		t.Fatalf("%+v", problems)
	}
	if len(c.Entries) != 1 || len(c.Grid) != 10 || c.Baseline == "" || len(c.Gaps) != 1 || c.Entries[0].Facts["Price"] != "subscription" {
		t.Fatalf("%+v", c)
	}
	r := Check(map[string][]byte{VoicesFile: []byte(goodVoices), CompetitorsFile: []byte(goodCompetitors)}, "research", today)
	for _, d := range r.Diagnostics {
		if d.Rule != RuleBalance {
			t.Errorf("unexpected finding: %s", d.String())
		}
	}
	if r.Summary.Voices != 2 || r.Summary.Peers != 1 || len(r.Summary.Platforms) != 2 || r.Summary.Dated != 1 || r.Summary.Recent != 1 {
		t.Fatalf("summary: %+v", r.Summary)
	}
}

func TestCRLFAndTrailingWhitespaceAreTolerated(t *testing.T) {
	if _, problems := ParseVoices(strings.ReplaceAll(goodVoices, "\n", "\r\n")); len(problems) != 0 {
		t.Fatalf("%+v", problems)
	}
}

func TestFormatProblems(t *testing.T) {
	for name, tc := range map[string]struct{ text, want string }{
		"title":      {strings.Replace(goodVoices, "# Human voices: Night shift sleep", "# Voices", 1), "must start with"},
		"batch date": {strings.Replace(goodVoices, "Batch 2026-10-17", "Batch 17.10.2026", 1), "is not YYYY-MM-DD"},
		"mode":       {strings.Replace(goodVoices, "· from zero ·", "· first pass ·", 1), "neither"},
		"queries":    {strings.Replace(goodVoices, ` · unreachable: Instagram (login wall)`, "", 1), "lacks its queries"},
		"fields":     {strings.Replace(goodVoices, " · via fetch", "", 1), "does not match the format"},
		"author":     {strings.Replace(goodVoices, "· buyer ·", "· nurse ·", 1), "Author type"},
		"posted":     {strings.Replace(goodVoices, "posted 2026-03-02", "posted March 2026", 1), "Posted date"},
		"via":        {strings.Replace(goodVoices, "via fetch", "via memory", 1), "Fetch method"},
		"id":         {strings.Replace(goodVoices, "### V001", "### V1", 1), "is not V followed"},
		"no quote":   {strings.Replace(goodVoices, "> POV: you're asleep at 3pm and the doorbell rings\n", "", 1), "has no quote"},
		"no url":     {strings.Replace(goodVoices, "<https://www.tiktok.com/@nurse/video/123>\n", "", 1), "no source URL"},
		"bad url":    {strings.Replace(goodVoices, "<https://www.tiktok.com/@nurse/video/123>", "<tiktok>", 1), "not an absolute web URL"},
		"paraphrase": {strings.Replace(goodVoices, "> POV:", "POV:", 1), "Unexpected line"},
		"sequence":   {strings.Replace(goodVoices, "### V002", "### V003", 1), "Expected V002"},
	} {
		t.Run(name, func(t *testing.T) {
			if !found(Check(map[string][]byte{VoicesFile: []byte(tc.text)}, "research", today), RuleFormat, tc.want) {
				t.Fatalf("no %s finding containing %q", RuleFormat, tc.want)
			}
		})
	}
	for name, tc := range map[string]struct{ text, want string }{
		"fact missing": {strings.Replace(goodCompetitors, "- Tone: calm\n", "", 1), "lacks Tone"},
		"unknown fact": {strings.Replace(goodCompetitors, "- Tone: calm", "- Mood: calm", 1), "Unknown peer fact"},
		"merged facts": {strings.Replace(goodCompetitors, "- Palette: navy, white", "- Palette: navy · Type: sans", 1), "also holds Type"},
		"grid row":     {strings.Replace(goodCompetitors, "| Tone | calm | 1 |", "| Tone | calm | one |", 1), "Grid row"},
		"no baseline":  {strings.Replace(goodCompetitors, "Navy app pages that promise better sleep.\n", "", 1), "without a baseline"},
		"peer url":     {strings.Replace(goodCompetitors, "https://example.com/sleep", "example.com", 1), "not an absolute web URL"},
	} {
		t.Run(name, func(t *testing.T) {
			if !found(Check(map[string][]byte{CompetitorsFile: []byte(tc.text)}, "research", today), RuleFormat, tc.want) {
				t.Fatalf("no %s finding containing %q", RuleFormat, tc.want)
			}
		})
	}
}

func TestSubstanceProblems(t *testing.T) {
	for name, tc := range map[string]struct{ file, text, want string }{
		"future fetch":  {VoicesFile, strings.Replace(goodVoices, "fetched 2026-10-18", "fetched 2026-12-01", 1), "in the future"},
		"order":         {VoicesFile, strings.Replace(goodVoices, "posted 2026-03-02", "posted 2026-10-19", 1), "Posted after it was fetched"},
		"stale fetch":   {VoicesFile, strings.Replace(goodVoices, "fetched 2026-10-18", "fetched 2025-01-01", 1), "outside its batch"},
		"placeholder":   {VoicesFile, strings.Replace(goodVoices, "> POV: you're asleep", "> <exact text>", 1), "placeholder"},
		"duplicate":     {VoicesFile, strings.Replace(goodVoices, "> POV: you're asleep at 3pm and the doorbell rings", "> Blackout curtains changed my life.\n>\n> ### This line is part of the quote · not a heading", 1), "Same quote as V001"},
		"grid outdated": {CompetitorsFile, strings.Replace(goodCompetitors, "| Tone | calm | 1 |", "| Tone | calm | 3 |", 1), "Grid counts 3 peers for Tone"},
	} {
		t.Run(name, func(t *testing.T) {
			if !found(Check(map[string][]byte{tc.file: []byte(tc.text)}, "research", today), RuleSubstance, tc.want) {
				t.Fatalf("no %s finding containing %q", RuleSubstance, tc.want)
			}
		})
	}
}

func TestBalance(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Human voices: X\n\n## Batch 2026-10-17 · from zero · queries: \"x\" · unreachable: none\n\n")
	for i := 1; i <= 20; i++ {
		platform := "Reddit"
		if i > 15 {
			platform = "Forum"
		}
		fmt.Fprintf(&b, "### V%03d · %s · buyer · posted 2026-05-01 · fetched 2026-10-17 · via fetch\n> voice number %d\n<https://example.com/%d>\n\n", i, platform, i, i)
	}
	r := Check(map[string][]byte{VoicesFile: []byte(b.String())}, "research", today)
	for _, want := range []string{"20 voices from 2 platforms", "come from reddit", "posted on 2026-05-01"} {
		if !found(r, RuleBalance, want) {
			t.Errorf("no balance finding containing %q", want)
		}
	}
	ongoing := strings.Replace(b.String(), "· from zero ·", "· ongoing: new reviews ·", 1)
	if r := Check(map[string][]byte{VoicesFile: []byte(ongoing)}, "research", today); found(r, RuleBalance, "voices from") || found(r, RuleBalance, "come from") || found(r, RuleBalance, "posted on") {
		t.Error("balance applies only once research started from zero")
	}
}

func TestResearchNeedsBothFilesAndCalendarDates(t *testing.T) {
	voices := "# Human voices: X\n\n## Batch 2026-10-18 · ongoing: launch · queries: \"q\" · unreachable: none\n\n### V001 · Reddit · buyer · posted 2026-10-01 · fetched 2026-10-18 · via fetch\n> a voice\n<https://example.com/1>\n"
	peersOnlyHeading := "# Competitor analysis: X\n\n## Batch 2026-10-18 · from zero\n"
	// today is a moment late on the 17th in UTC; the 18th is already today further east.
	late := time.Date(2026, 10, 17, 23, 30, 0, 0, time.UTC)
	r := Check(map[string][]byte{VoicesFile: []byte(voices), CompetitorsFile: []byte("# Competitor analysis: X\n")}, "research", late)
	if found(r, RuleSubstance, "future") {
		t.Error("a fetch dated tomorrow in UTC is today somewhere:", r.Diagnostics)
	}
	if !found(r, RuleBalance, "no competitor") {
		t.Error("voices without peers must be reported:", r.Diagnostics)
	}
	if r := Check(map[string][]byte{CompetitorsFile: []byte(peersOnlyHeading)}, "research", late); !found(r, RuleBalance, "0 peers") {
		t.Error("an empty from-zero competitor batch must report its minimum:", r.Diagnostics)
	}
	if r := Check(map[string][]byte{VoicesFile: []byte(strings.ReplaceAll(voices, "2026-10-18", "2026-10-20"))}, "research", late); !found(r, RuleSubstance, "future") {
		t.Error("two days ahead is the future everywhere")
	}
}

func TestStarterFilesAreClean(t *testing.T) {
	r := Check(map[string][]byte{VoicesFile: []byte("# Human voices: Tea shop\n"), CompetitorsFile: []byte("# Competitor analysis: Tea shop\n")}, "research", today)
	if len(r.Diagnostics) != 0 || !r.Summary.Empty() {
		t.Fatalf("%+v", r)
	}
}

func FuzzBlockquoteRoundTrip(f *testing.F) {
	f.Add("plain")
	f.Add("### V001 · heading-like\n\n> nested\n<https://x>\nObserver note: fake")
	f.Add(" leading space\n\ttab\n")
	f.Fuzz(func(t *testing.T, text string) {
		text = strings.ReplaceAll(text, "\r", "")
		file := "# Human voices: X\n\n## Batch 2026-10-17 · from zero · queries: \"q\" · unreachable: none\n\n" +
			"### V001 · Reddit · buyer · posted unknown · fetched 2026-10-17 · via fetch\n" + Blockquote(text) + "\n<https://example.com/a>\n"
		v, _ := ParseVoices(file)
		if len(v.Entries) != 1 || v.Entries[0].Quote != text {
			t.Fatalf("verbatim text did not survive: %q", text)
		}
	})
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add(goodVoices)
	f.Add(goodCompetitors)
	f.Fuzz(func(t *testing.T, text string) {
		n := len(lines(text))
		for _, p := range append(func() []Problem { _, p := ParseVoices(text); return p }(), func() []Problem { _, p := ParseCompetitors(text); return p }()...) {
			if p.Line < 1 || p.Line > n {
				t.Fatalf("problem at line %d of %d", p.Line, n)
			}
		}
		Check(map[string][]byte{VoicesFile: []byte(text), CompetitorsFile: []byte(text)}, "research", today)
	})
}

func found(r Report, rule, text string) bool {
	for _, d := range r.Diagnostics {
		if d.Rule == rule && strings.Contains(d.Message, text) {
			return true
		}
	}
	return false
}

func TestPrecisionShortfallAndAdjacent(t *testing.T) {
	text := strings.Replace(goodVoices, "posted 2026-03-02", "posted 2021", 1)
	text = strings.Replace(text, "· creator ·", "· adjacent ·", 1)
	text = strings.Replace(text, "unreachable: Instagram (login wall)", "unreachable: Instagram (login wall) · shortfall: niche has few public voices", 1)
	v, problems := ParseVoices(text)
	if len(problems) != 0 || v.Entries[0].Precision != Year || v.Batches[0].Shortfall != "niche has few public voices" {
		t.Fatalf("%+v %+v", problems, v.Entries[0])
	}
	if found(Check(map[string][]byte{VoicesFile: []byte(text)}, "research", today), RuleBalance, "research from zero needs") {
		t.Error("a named shortfall acknowledges the minimums")
	}
	if found(Check(map[string][]byte{VoicesFile: []byte(strings.Replace(text, "posted 2021", "posted 2026-10", 1))}, "research", today), RuleSubstance, "") {
		t.Error("a month that includes the fetch date is plausible")
	}
}
