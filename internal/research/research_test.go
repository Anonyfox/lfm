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
	if r := Check(map[string][]byte{VoicesFile: []byte(ongoing)}, "research", today); !found(r, RuleBalance, "voices from") || !found(r, RuleBalance, "come from") || !found(r, RuleBalance, "posted on") {
		t.Error("balance applies across all batches")
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
	f.Add(goodVoices + "Imported: 2026-10-17 from home V014\n")
	f.Add(strings.Replace(goodCompetitors, "- Opening:", "Imported: 2026-10-17 from home P003\n- Opening:", 1))
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
	if found(Check(map[string][]byte{VoicesFile: []byte(text)}, "research", today), RuleBalance, "project research needs") {
		t.Error("a named shortfall acknowledges the minimums")
	}
	if found(Check(map[string][]byte{VoicesFile: []byte(strings.Replace(text, "posted 2021", "posted 2026-10", 1))}, "research", today), RuleSubstance, "") {
		t.Error("a month that includes the fetch date is plausible")
	}
}

func TestProjectMinimumsAndShortfall(t *testing.T) {
	for _, mode := range []string{"from zero", "ongoing: refresh"} {
		v := strings.ReplaceAll(goodVoices, "from zero", mode)
		c := strings.ReplaceAll(goodCompetitors, "from zero", mode)
		files := map[string][]byte{VoicesFile: []byte(v), CompetitorsFile: []byte(c)}
		r := Check(files, "research", today)
		if !found(r, RuleBalance, "2 voices") || !found(r, RuleBalance, "1 peers") {
			t.Fatalf("missing project minimums: %+v", r.Diagnostics)
		}
		for _, file := range []string{VoicesFile, CompetitorsFile} {
			note := "only two public discussions"
			text := string(files[file])
			lines := strings.Split(text, "\n")
			for i := range lines {
				if strings.HasPrefix(lines[i], "## Batch ") {
					lines[i] += " · shortfall: " + note
					break
				}
			}
			waived := map[string][]byte{VoicesFile: files[VoicesFile], CompetitorsFile: files[CompetitorsFile]}
			waived[file] = []byte(strings.Join(lines, "\n"))
			r := Check(waived, "research", today)
			if found(r, RuleBalance, "project research needs") || len(r.Summary.Shortfalls) != 1 || r.Summary.Shortfalls[0] != note {
				t.Fatalf("waiver not shared or visible: %+v", r)
			}
		}
	}
}

func TestImports(t *testing.T) {
	v := strings.Replace(goodVoices, "fetched 2026-10-18", "fetched 2025-01-01", 1)
	v += "Imported: 2026-10-17 from home V014\n"
	c := strings.Replace(goodCompetitors, "fetched 2026-10-17", "fetched 2025-01-01", 1)
	c = strings.Replace(c, "- Opening:", "Imported: 2026-10-17 from home P003\n- Opening:", 1)
	files := map[string][]byte{VoicesFile: []byte(v), CompetitorsFile: []byte(c)}
	r := Check(files, "research", today)
	if found(r, RuleFormat, "") || found(r, RuleSubstance, "outside its batch") {
		t.Fatalf("valid imports: %+v", r.Diagnostics)
	}
	if r.Summary.Voices != 2 || r.Summary.Peers != 1 || r.Voices.Entries[1].Imported.Origin != "home V014" {
		t.Fatalf("imports not counted: %+v", r)
	}
	for _, line := range []string{"Imported: nonsense", "Imported: 2026-10-17 from ", "Imported: 2026-02-30 from home V014"} {
		if _, p := parseImport(line, 1); p == nil {
			t.Errorf("accepted %q", line)
		}
	}
	for _, tc := range []struct{ old, replacement, want string }{
		{"Imported: 2026-10-17", "Imported: 2026-12-01", "future"},
		{"Imported: 2026-10-17", "Imported: 2024-01-01", "Fetched after"},
		{"> POV: you're asleep at 3pm and the doorbell rings", "> Blackout curtains changed my life.\n>\n> ### This line is part of the quote · not a heading", "Same quote"},
	} {
		files[VoicesFile] = []byte(strings.Replace(v, tc.old, tc.replacement, 1))
		if !found(Check(files, "research", today), RuleSubstance, tc.want) {
			t.Errorf("missing %s", tc.want)
		}
	}
}

func TestRefreshUsesTotals(t *testing.T) {
	var v, c strings.Builder
	v.WriteString("# Human voices: X\n## Batch 2026-10-17 · ongoing: imported sample · queries: x · unreachable: none\n")
	c.WriteString("# Competitor analysis: X\n## Batch 2026-10-17 · ongoing: imported sample\n")
	for i := 1; i <= 61; i++ {
		if i == 61 {
			v.WriteString("## Batch 2026-10-18 · ongoing: new question · queries: x · unreachable: none\n")
		}
		fmt.Fprintf(&v, "### V%03d · Platform%d · buyer · posted unknown · fetched 2026-10-17 · via fetch\n> unique %d\n<https://example.com/v%d>\n", i, i%5, i, i)
	}
	for i := 1; i <= 13; i++ {
		if i == 13 {
			c.WriteString("## Batch 2026-10-18 · ongoing: new comparison\n")
		}
		fmt.Fprintf(&c, "### P%03d · Peer · https://example.com/p%d · fetched 2026-10-17 · via fetch\n", i, i)
		for _, field := range PeerFields {
			fmt.Fprintf(&c, "- %s: none\n", field)
		}
	}
	r := Check(map[string][]byte{VoicesFile: []byte(v.String()), CompetitorsFile: []byte(c.String())}, "research", today)
	if found(r, RuleBalance, "") {
		t.Fatalf("refresh imposed separate minimums: %+v", r.Diagnostics)
	}
}

func TestImportFormatAndDuplicatePeers(t *testing.T) {
	for _, text := range []string{
		strings.Replace(goodCompetitors, "- Opening:", "Imported: invalid\n- Opening:", 1),
		strings.Replace(goodCompetitors, "- Opening:", "Imported: 2026-10-17 from home P001\nImported: 2026-10-17 from home P001\n- Opening:", 1),
	} {
		if !found(Check(map[string][]byte{CompetitorsFile: []byte(text)}, "research", today), RuleFormat, "") {
			t.Fatal("accepted invalid import")
		}
	}
	c := strings.Replace(goodCompetitors, "### Grid after this batch", "### P002 · Copy · https://example.com/sleep · fetched 2026-10-17 · via fetch\nImported: 2026-10-17 from home P001\n\n### Grid after this batch", 1)
	if !found(Check(map[string][]byte{CompetitorsFile: []byte(c)}, "research", today), RuleSubstance, "Same page as P001") {
		t.Fatal("import hid duplicate peer")
	}
}
