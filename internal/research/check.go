package research

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// Rule IDs reported by Check.
const (
	RuleFormat    = "research.format"
	RuleSubstance = "research.substance"
	RuleBalance   = "research.balance"
)

// Rules describes the research checks for the rule catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{
			ID:      RuleFormat,
			Summary: "Research files follow the exact format of the lfm-research skill",
			Help:    "Every heading, field and ID in research/human-voices.md and research/competitor-analysis.md is parsed: batch headings, voice and peer headings, quote blocks, source URLs, peer facts and the grid table. A malformed entry cannot be cited reliably and usually means text was paraphrased or merged. Fix the named line to match the format printed by lfm skills research; quote lines always start with \">\".",
		},
		{
			ID:      RuleSubstance,
			Summary: "Research entries are plausible: real dates, no duplicates, no template placeholders",
			Help:    "Flags dates in the future or out of order, sources fetched outside their batch, the same quote recorded twice, the same competitor twice, leftover template placeholders and a competitor grid that no longer adds up to the number of peers. These are the typical traces of copied, invented or stale entries. Correct the entry from its source, delete the duplicate in its own batch only if it was added in this session, or recount the grid.",
		},
		{
			ID:      RuleBalance,
			Summary: "Research from zero meets its minimums and is not dominated by one source",
			Help:    "Once a from-zero batch exists, the files need 60 voices from 5 platforms and 12 competitor pages, no platform above 40% of the voices, and no single day holding most of the dated voices, which usually means one thread was copied wholesale. A narrow sample makes every later design decision rest on one corner of the market. Collect more from underrepresented platforms, or name the shortfall in the batch heading and set this rule to off if the market is truly that small.",
		},
	}
}

// Report is the result of checking a project's research.
type Report struct {
	Diagnostics []diagnostic.Diagnostic
	Summary     Summary
	Voices      *Voices      // nil when the file is absent
	Competitors *Competitors // nil when the file is absent
}

// Check parses and checks the research files present in files (keyed by
// VoicesFile and CompetitorsFile; absent files are skipped). dir prefixes the
// file names in diagnostics; today bounds plausible dates.
func Check(files map[string][]byte, dir string, today time.Time) Report {
	var r Report
	report := func(file, rule string, line int, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{
			Rule: rule, Severity: diagnostic.Warning,
			Location: diagnostic.Location{File: dir + "/" + file, Line: line, Column: 1},
			Message:  message, Hint: hint,
		})
	}
	var voices *Voices
	var competitors *Competitors
	if raw, ok := files[VoicesFile]; ok {
		v, problems := ParseVoices(string(raw))
		for _, p := range problems {
			report(VoicesFile, RuleFormat, p.Line, p.Message, p.Hint)
		}
		checkVoices(v, today, func(rule string, line int, message, hint string) { report(VoicesFile, rule, line, message, hint) })
		voices = &v
	}
	if raw, ok := files[CompetitorsFile]; ok {
		c, problems := ParseCompetitors(string(raw))
		for _, p := range problems {
			report(CompetitorsFile, RuleFormat, p.Line, p.Message, p.Hint)
		}
		checkCompetitors(c, today, func(rule string, line int, message, hint string) { report(CompetitorsFile, rule, line, message, hint) })
		competitors = &c
	}
	// Each file is checked alone above; research from zero needs both, so a
	// filled file beside an empty or missing one is reported at the empty one.
	hasVoices := voices != nil && len(voices.Entries) > 0
	hasPeers := competitors != nil && len(competitors.Entries) > 0
	if hasVoices && !hasPeers {
		report(CompetitorsFile, RuleBalance, 1, "Voices are collected but no competitor is analyzed yet.", "Visit the pages buyers compare against and record them; design needs both files.")
	}
	if hasPeers && !hasVoices {
		report(VoicesFile, RuleBalance, 1, "Competitors are analyzed but no voice is collected yet.", "Collect verbatim buyer voices; design needs both files.")
	}
	r.Summary = summarize(voices, competitors)
	r.Voices, r.Competitors = voices, competitors
	return r
}

type reporter func(rule string, line int, message, hint string)

func checkSequence(ids []int, lines []int, prefix string, add reporter) {
	seen := map[int]bool{}
	for k, id := range ids {
		switch {
		case seen[id]:
			add(RuleFormat, lines[k], fmt.Sprintf("%s%03d is used twice.", prefix, id), "IDs are never reused; give the later entry the next free number.")
		case id != k+1:
			add(RuleFormat, lines[k], fmt.Sprintf("Expected %s%03d here, found %s%03d.", prefix, k+1, prefix, id), "IDs continue without gaps across batches and are never renumbered once cited.")
		}
		seen[id] = true
	}
}

// checkDates reports implausible posted/fetched dates relative to the batch.
// A posted period counts as plausible if its first day is.
func checkDates(posted, fetched, batch, today time.Time, line int, add reporter) {
	// Dates are calendar days. A source opened "today" somewhere east of here
	// is already tomorrow, so the limit is the end of tomorrow.
	y, m, d := today.Date()
	today = time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if !fetched.IsZero() && fetched.After(today) {
		add(RuleSubstance, line, "Fetched date "+fetched.Format(time.DateOnly)+" is in the future.", "Use the day the source was opened.")
	}
	if !posted.IsZero() && posted.After(today) {
		add(RuleSubstance, line, "Posted date is in the future.", "Copy the date the source shows, or write unknown.")
	}
	if !posted.IsZero() && !fetched.IsZero() && posted.After(fetched) {
		add(RuleSubstance, line, "Posted after it was fetched.", "One of the two dates is wrong; check the source.")
	}
	if !batch.IsZero() && !fetched.IsZero() && (fetched.Before(batch.AddDate(0, 0, -1)) || fetched.After(batch.AddDate(0, 0, 7))) {
		add(RuleSubstance, line, "Fetched "+fetched.Format(time.DateOnly)+", outside its batch of "+batch.Format(time.DateOnly)+".", "Entries are fetched during their batch; a stale fetched date usually means the entry was copied from elsewhere.")
	}
}

// placeholders are template fragments that must never survive into a file.
var placeholders = []string{"<exact text>", "<platform>", "<place>", "<project>", "<name>", "https://source.url", "https://peer.url", "<query>", "<trigger>"}

func placeholder(s string) string {
	for _, p := range placeholders {
		if strings.Contains(s, p) {
			return p
		}
	}
	return ""
}

func checkVoices(v Voices, today time.Time, add reporter) {
	ids, lines := make([]int, len(v.Entries)), make([]int, len(v.Entries))
	quotes := map[string]int{}
	for k, e := range v.Entries {
		ids[k], lines[k] = e.ID, e.Line
		var batch time.Time
		if e.Batch >= 0 && e.Batch < len(v.Batches) {
			batch = v.Batches[e.Batch].Date
		}
		checkDates(e.Posted, e.Fetched, batch, today, e.Line, add)
		if p := placeholder(e.Quote + " " + e.Platform + " " + e.Place + " " + e.URL); p != "" {
			add(RuleSubstance, e.Line, "Template placeholder "+p+" left in an entry.", "Replace it with the real text or source, or remove the entry.")
		}
		key := strings.Join(strings.Fields(strings.ToLower(e.Quote)), " ")
		if first, seen := quotes[key]; seen && key != "" {
			add(RuleSubstance, e.Line, fmt.Sprintf("Same quote as V%03d.", first), "Keep the original source once; reposts are not new voices.")
		} else {
			quotes[key] = e.ID
		}
	}
	checkSequence(ids, lines, "V", add)
	for _, b := range v.Batches {
		if p := placeholder(b.Queries + " " + b.Unreachable + " " + b.Trigger); p != "" {
			add(RuleSubstance, b.Line, "Template placeholder "+p+" left in a batch heading.", "Write the actual queries and unreachable platforms.")
		}
	}
	if !fromZero(v.Batches) {
		return
	}
	platforms := map[string]int{}
	days := map[time.Time]int{}
	dated := 0
	for _, e := range v.Entries {
		platforms[strings.ToLower(e.Platform)]++
		if e.Precision == Day {
			days[e.Posted]++
			dated++
		}
	}
	total := len(v.Entries)
	if (total < MinVoices || len(platforms) < MinPlatforms) && !shortfall(v.Batches) {
		add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d voices from %d platforms; research from zero needs %d from %d.", total, len(platforms), MinVoices, MinPlatforms), "Collect more, preferring platforms not yet covered, or add \"· shortfall: <what and why>\" to the batch heading.")
	}
	if total >= 10 {
		for _, name := range sortedKeys(platforms) {
			if n := platforms[name]; n*5 > total*2 {
				add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d of %d voices (%d%%) come from %s.", n, total, 100*n/total, name), "Keep any one platform at or below 40% so one corner of the market cannot dominate.")
			}
		}
	}
	if dated >= 10 {
		order := make([]time.Time, 0, len(days))
		for day := range days {
			order = append(order, day)
		}
		sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
		for _, day := range order {
			if n := days[day]; n*10 > dated*3 {
				add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d of %d dated voices were posted on %s.", n, dated, day.Format(time.DateOnly)), "One thread or campaign dominates; spread across sources and time.")
			}
		}
	}
}

func checkCompetitors(c Competitors, today time.Time, add reporter) {
	ids, lines := make([]int, len(c.Entries)), make([]int, len(c.Entries))
	urls := map[string]int{}
	for k, p := range c.Entries {
		ids[k], lines[k] = p.ID, p.Line
		var batch time.Time
		if p.Batch >= 0 && p.Batch < len(c.Batches) {
			batch = c.Batches[p.Batch].Date
		}
		checkDates(time.Time{}, p.Fetched, batch, today, p.Line, add)
		var missing []string
		for _, field := range PeerFields {
			if p.Facts[field] == "" {
				missing = append(missing, field)
			}
		}
		if len(missing) > 0 {
			add(RuleFormat, p.Line, fmt.Sprintf("P%03d lacks %s.", p.ID, strings.Join(missing, ", ")), "Record every fact on its own \"- Fact: value\" line; write none when the page shows nothing.")
		}
		facts := ""
		for _, v := range p.Facts {
			facts += " " + v
		}
		if ph := placeholder(p.Name + " " + p.URL + facts); ph != "" {
			add(RuleSubstance, p.Line, "Template placeholder "+ph+" left in an entry.", "Replace it with what the page shows.")
		}
		key := strings.TrimSuffix(strings.ToLower(p.URL), "/")
		if first, seen := urls[key]; seen {
			add(RuleSubstance, p.Line, fmt.Sprintf("Same page as P%03d.", first), "Analyze each competitor once; later changes belong in an ongoing batch as a new entry only if the page changed.")
		} else {
			urls[key] = p.ID
		}
	}
	checkSequence(ids, lines, "P", add)
	if fromZero(c.Batches) && len(c.Entries) < MinPeers {
		add(RuleBalance, c.Batches[0].Line, fmt.Sprintf("%d peers; research from zero needs %d.", len(c.Entries), MinPeers), "Add pages the buyer compares against, including some that rank in search.")
	}
	if len(c.Entries) == 0 {
		return
	}
	if len(c.Grid) == 0 {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without a grid.", "Add \"### Grid after this batch\" with one | Fact | Value | Peers | row per value.")
	} else {
		sums := map[string]int{}
		for _, row := range c.Grid {
			sums[row.Column] += row.Peers
		}
		for _, field := range PeerFields {
			if sums[field] != len(c.Entries) {
				add(RuleSubstance, c.GridLine, fmt.Sprintf("Grid counts %d peers for %s; the file has %d.", sums[field], field, len(c.Entries)), "Recount the grid across all peers after every batch.")
			}
		}
	}
	if strings.TrimSpace(c.Baseline) == "" {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without a baseline.", "Add \"### Baseline\" with one paragraph naming the most frequent value per fact.")
	}
	if len(c.Gaps) == 0 {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without gaps.", "Add \"### Gaps\" listing values no peer uses.")
	}
}

// shortfall reports whether a batch acknowledges unmet minimums.
func shortfall(batches []Batch) bool {
	for _, b := range batches {
		if b.Shortfall != "" {
			return true
		}
	}
	return false
}

func fromZero(batches []Batch) bool {
	for _, b := range batches {
		if b.FromZero {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
