// Package research reads a project's research files, checks their format and
// substance, and summarizes them. The format is the one the lfm-research
// skill writes; the skill's own examples are tested against this parser.
//
// Structure lives only in lines that start with a heading marker, a field
// marker or a table pipe. Quotes are blockquotes: every line starts with ">",
// so verbatim text may contain anything, including "#", "·" and URLs.
package research

import (
	"net/url"
	"strings"
	"time"
)

// Files inside the project's research directory.
const (
	VoicesFile      = "human-voices.md"
	CompetitorsFile = "competitor-analysis.md"
)

// Minimums for research started from zero.
const (
	MinVoices    = 60
	MinPlatforms = 5
	MinPeers     = 12
)

// Fixed vocabularies.
var (
	Authors = []string{"buyer", "adjacent", "creator", "brand", "expert"}
	Methods = []string{"connector", "fetch", "browser", "user"}
	// PeerFields are the facts recorded for every competitor, in order.
	PeerFields = []string{"Opening", "Palette", "Type", "Imagery", "Motion", "Mechanism", "Proof", "Price", "Tone", "Takeaway"}
)

const sep = " · "

// Problem is a format defect at a line of a research file.
type Problem struct {
	Line    int
	Message string
	Hint    string
}

// Batch is one dated research session.
type Batch struct {
	Line        int
	Date        time.Time
	FromZero    bool
	Trigger     string // ongoing batches name what triggered them
	Queries     string
	Unreachable string
	Shortfall   string // why minimums are not met, when they are not
}

// Voice is one verbatim quote.
type Voice struct {
	Line      int
	ID        int
	Platform  string
	Place     string
	Author    string
	Posted    time.Time // start of the posted period; zero when the source shows no date
	Precision Precision
	Fetched   time.Time
	Via       string
	Quote     string
	URL       string
	Observer  string
	Batch     int // index into Voices.Batches
}

// Precision is how exactly a source dates a post.
type Precision int

const (
	Unknown Precision = iota
	Year
	Month
	Day
)

// postedDate reads YYYY-MM-DD, YYYY-MM or YYYY.
func postedDate(s string) (time.Time, Precision, bool) {
	for _, f := range []struct {
		layout    string
		precision Precision
	}{{time.DateOnly, Day}, {"2006-01", Month}, {"2006", Year}} {
		if len(s) == len(f.layout) {
			if t, err := time.Parse(f.layout, s); err == nil {
				return t, f.precision, true
			}
		}
	}
	return time.Time{}, Unknown, false
}

// Voices is human-voices.md.
type Voices struct {
	Title   string
	Batches []Batch
	Entries []Voice
}

// Peer is one competitor page.
type Peer struct {
	Line    int
	ID      int
	Name    string
	URL     string
	Fetched time.Time
	Via     string
	Facts   map[string]string
	Batch   int
}

// GridRow is one value of one column and the number of peers showing it.
type GridRow struct {
	Line   int
	Column string
	Value  string
	Peers  int
}

// Competitors is competitor-analysis.md. Grid, Baseline and Gaps are those of
// the latest batch that has them.
type Competitors struct {
	Title    string
	Batches  []Batch
	Entries  []Peer
	Grid     []GridRow
	GridLine int
	Baseline string
	Gaps     []string
}

// lines splits text into lines without line endings, numbered from 1.
func lines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func date(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func oneOf(s string, list []string) bool {
	for _, item := range list {
		if s == item {
			return true
		}
	}
	return false
}

// webURL accepts absolute http(s) URLs with a host.
func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " <>")
}

// parseBatch reads "## Batch <date> · <mode>[ · queries: … · unreachable: …]".
func parseBatch(line string, n int, withSources bool) (Batch, *Problem) {
	format := "## Batch YYYY-MM-DD · from zero | ongoing: <trigger>"
	if withSources {
		format += ` · queries: "…" · unreachable: <platforms or none>`
	}
	bad := &Problem{Line: n, Message: "Batch heading does not match the format.", Hint: "Write it as: " + format}
	rest, ok := strings.CutPrefix(line, "## Batch ")
	if !ok {
		return Batch{}, bad
	}
	fields := strings.Split(rest, sep)
	if len(fields) < 2 {
		return Batch{}, bad
	}
	b := Batch{Line: n}
	if b.Date, ok = date(fields[0]); !ok {
		return Batch{}, &Problem{Line: n, Message: "Batch date " + quote(fields[0]) + " is not YYYY-MM-DD.", Hint: "Use the date of the research session, e.g. 2026-10-17."}
	}
	switch mode := fields[1]; {
	case mode == "from zero":
		b.FromZero = true
	case strings.HasPrefix(mode, "ongoing: ") && strings.TrimSpace(mode[len("ongoing: "):]) != "":
		b.Trigger = strings.TrimSpace(mode[len("ongoing: "):])
	default:
		return Batch{}, &Problem{Line: n, Message: "Batch mode " + quote(mode) + " is neither \"from zero\" nor \"ongoing: <trigger>\".", Hint: "Name the mode; an ongoing batch says what triggered it."}
	}
	if !withSources {
		if len(fields) != 2 {
			return Batch{}, bad
		}
		return b, nil
	}
	tail := strings.Join(fields[2:], sep)
	queries, unreachable, found := strings.Cut(tail, sep+"unreachable: ")
	unreachable, shortfall, hasShortfall := strings.Cut(unreachable, sep+"shortfall: ")
	if hasShortfall {
		if b.Shortfall = strings.TrimSpace(shortfall); b.Shortfall == "" {
			return Batch{}, &Problem{Line: n, Message: "Empty shortfall in the batch heading.", Hint: "Say what is missing and why, or drop the field."}
		}
	}
	queries, hasQueries := strings.CutPrefix(queries, "queries: ")
	if !found || !hasQueries || strings.TrimSpace(queries) == "" || strings.TrimSpace(unreachable) == "" {
		return Batch{}, &Problem{Line: n, Message: "Voice batch heading lacks its queries or unreachable list.", Hint: "List the searches and the platforms that could not be reached (or none): " + format}
	}
	b.Queries, b.Unreachable = strings.TrimSpace(queries), strings.TrimSpace(unreachable)
	return b, nil
}

// entryID reads "V012" or "P003".
func entryID(field, prefix string) (int, bool) {
	digits, ok := strings.CutPrefix(field, prefix)
	if !ok || len(digits) < 3 || len(digits) > 6 {
		return 0, false
	}
	id := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		id = id*10 + int(c-'0')
	}
	return id, id > 0
}

// ParseVoices reads human-voices.md. It never stops at a problem, so every
// defect in a file is reported at once.
func ParseVoices(text string) (Voices, []Problem) {
	var v Voices
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	ls := lines(text)
	if title, ok := strings.CutPrefix(ls[0], "# Human voices: "); ok && strings.TrimSpace(title) != "" {
		v.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Human voices: <project>\".", "Restore the title line; the rest of the format hangs off it.")
	}
	entryFormat := "### V001 · Platform (place) · " + strings.Join(Authors, "|") + " · posted YYYY-MM-DD|YYYY-MM|YYYY|unknown · fetched YYYY-MM-DD · via " + strings.Join(Methods, "|")
	for i := 1; i < len(ls); i++ {
		line, n := ls[i], i+1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "## "):
			if b, p := parseBatch(line, n, true); p != nil {
				problems = append(problems, *p)
			} else {
				v.Batches = append(v.Batches, b)
			}
		case strings.HasPrefix(line, "|"):
			if len(v.Batches) == 0 {
				add(n, "Table outside a batch.", "Put behaviour numbers in a table directly under their batch heading.")
			}
		case strings.HasPrefix(line, "### "):
			entry, next, entryProblems := parseVoice(ls, i, entryFormat)
			problems = append(problems, entryProblems...)
			if entry.ID > 0 {
				if len(v.Batches) == 0 {
					add(n, "Voice before the first batch heading.", "Every voice belongs to a dated batch.")
				}
				entry.Batch = len(v.Batches) - 1
				v.Entries = append(v.Entries, entry)
			}
			i = next - 1
		default:
			add(n, "Unexpected line: "+preview(line), "Only batch headings, tables, voice headings, quote lines (\">\"), the source URL and an observer note belong here. Quote continuation lines start with \">\" too.")
		}
	}
	return v, problems
}

// parseVoice reads one voice starting at ls[i] and returns the index after it.
func parseVoice(ls []string, i int, format string) (Voice, int, []Problem) {
	n := i + 1
	var problems []Problem
	add := func(line int, message, hint string) { problems = append(problems, Problem{line, message, hint}) }
	v := Voice{Line: n}
	fields := strings.Split(strings.TrimPrefix(ls[i], "### "), sep)
	bad := func() { add(n, "Voice heading does not match the format.", "Write it as: "+format) }
	if len(fields) != 6 {
		bad()
	} else {
		var ok bool
		if v.ID, ok = entryID(fields[0], "V"); !ok {
			add(n, "Voice ID "+quote(fields[0])+" is not V followed by at least three digits.", "Continue the sequence: V001, V002, …")
		}
		v.Platform, v.Place = platform(fields[1])
		switch {
		case v.Platform == "":
			add(n, "Voice has no platform.", "Name the service it was posted on and, where it has one, the community: <platform> (<community>).")
		case strings.ContainsAny(v.Platform, "\"“”()") || len([]rune(v.Platform)) > 32:
			add(n, "Platform "+quote(v.Platform)+" reads like a place.", "Name the service only and put the thread, channel or page in parentheses: YouTube (\"Tips for nights\"). Otherwise platform counts break.")
		}
		if v.Author = fields[2]; !oneOf(v.Author, Authors) {
			add(n, "Author type "+quote(v.Author)+" is not one of "+strings.Join(Authors, ", ")+".", "Pick the closest; adjacent covers people close to the problem who would not buy.")
		}
		if posted, ok := strings.CutPrefix(fields[3], "posted "); !ok {
			bad()
		} else if posted != "unknown" {
			if v.Posted, v.Precision, ok = postedDate(posted); !ok {
				add(n, "Posted date "+quote(posted)+" is not YYYY-MM-DD, YYYY-MM, YYYY or unknown.", "Give the date as precisely as the source shows it: \"5 years ago\" becomes a year.")
			}
		}
		if fetched, ok := strings.CutPrefix(fields[4], "fetched "); !ok {
			bad()
		} else if v.Fetched, ok = date(fetched); !ok {
			add(n, "Fetched date "+quote(fetched)+" is not YYYY-MM-DD.", "Use the day you opened the source.")
		}
		if via, ok := strings.CutPrefix(fields[5], "via "); !ok || !oneOf(via, Methods) {
			add(n, "Fetch method "+quote(fields[5])+" is not via "+strings.Join(Methods, ", ")+".", "Say how the source was obtained.")
		} else {
			v.Via = via
		}
	}
	j := i + 1
	var quoteLines []string
	for ; j < len(ls) && strings.HasPrefix(ls[j], ">"); j++ {
		text := strings.TrimPrefix(ls[j], ">")
		quoteLines = append(quoteLines, strings.TrimPrefix(text, " "))
	}
	v.Quote = strings.Join(quoteLines, "\n")
	if strings.Trim(strings.TrimSpace(v.Quote), "…. ") == "" {
		add(n, "Voice has no quote.", "Copy the passage verbatim into lines that start with \">\".")
	}
	if j < len(ls) {
		raw := strings.TrimSpace(ls[j])
		if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
			v.URL = raw[1 : len(raw)-1]
			if !webURL(v.URL) {
				add(j+1, "Source "+quote(v.URL)+" is not an absolute web URL.", "Link the page where the quote can be reopened, in angle brackets.")
			}
			j++
		}
	}
	if v.URL == "" {
		add(n, "Voice has no source URL after its quote.", "Add the link on its own line, in angle brackets: <https://…>.")
	}
	if j < len(ls) {
		if note, ok := strings.CutPrefix(ls[j], "Observer note: "); ok {
			v.Observer = strings.TrimSpace(note)
			j++
		}
	}
	return v, j, problems
}

// platform splits "Reddit (r/pourover)" into its platform and place.
func platform(field string) (name, place string) {
	field = strings.TrimSpace(field)
	if open := strings.Index(field, " ("); open > 0 && strings.HasSuffix(field, ")") {
		return field[:open], field[open+2 : len(field)-1]
	}
	return field, ""
}

// ParseCompetitors reads competitor-analysis.md.
func ParseCompetitors(text string) (Competitors, []Problem) {
	var c Competitors
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	ls := lines(text)
	if title, ok := strings.CutPrefix(ls[0], "# Competitor analysis: "); ok && strings.TrimSpace(title) != "" {
		c.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Competitor analysis: <project>\".", "Restore the title line.")
	}
	section := ""
	var grid []GridRow
	var gridLine int
	var baseline []string
	var gaps []string
	flush := func() {
		if gridLine > 0 {
			c.Grid, c.GridLine = grid, gridLine
		}
		if len(baseline) > 0 {
			c.Baseline = strings.Join(baseline, " ")
		}
		if len(gaps) > 0 {
			c.Gaps = gaps
		}
		grid, gridLine, baseline, gaps = nil, 0, nil, nil
	}
	for i := 1; i < len(ls); i++ {
		line, n := ls[i], i+1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "## "):
			flush()
			section = ""
			if b, p := parseBatch(line, n, false); p != nil {
				problems = append(problems, *p)
			} else {
				c.Batches = append(c.Batches, b)
			}
		case line == "### Grid after this batch":
			section, gridLine = "grid", n
		case line == "### Baseline":
			section = "baseline"
		case line == "### Gaps":
			section = "gaps"
		case strings.HasPrefix(line, "### "):
			section = "peer"
			peer, p := parsePeerHeading(line, n)
			problems = append(problems, p...)
			if peer.ID > 0 {
				if len(c.Batches) == 0 {
					add(n, "Peer before the first batch heading.", "Every peer belongs to a dated batch.")
				}
				peer.Batch = len(c.Batches) - 1
				c.Entries = append(c.Entries, peer)
			}
		case section == "peer" && strings.HasPrefix(line, "- "):
			key, value, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
			if len(c.Entries) == 0 {
				continue
			}
			peer := &c.Entries[len(c.Entries)-1]
			switch {
			case !ok || !oneOf(key, PeerFields):
				add(n, "Unknown peer fact "+quote(key)+".", "Use one line per fact: "+strings.Join(PeerFields, ", ")+".")
			case peer.Facts[key] != "":
				add(n, "Peer fact "+key+" appears twice.", "Keep one line per fact.")
			case strings.TrimSpace(value) == "":
				add(n, "Peer fact "+key+" is empty.", "Record what the page shows, or none.")
			case mergedFact(value) != "":
				add(n, "Peer fact "+key+" also holds "+mergedFact(value)+".", "Put every fact on its own \"- Fact: value\" line.")
			default:
				peer.Facts[key] = strings.TrimSpace(value)
			}
		case section == "grid" && strings.HasPrefix(line, "|"):
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for k := range cells {
				cells[k] = strings.TrimSpace(cells[k])
			}
			if len(cells) != 3 {
				add(n, "Grid rows have three cells: column, value, peers.", "Write: | Opening | headline promise | 7 |")
				continue
			}
			if cells[0] == "Column" || strings.Trim(cells[0], "-: ") == "" {
				continue // header and separator rows
			}
			count, ok := entryCount(cells[2])
			if !oneOf(cells[0], PeerFields) || cells[1] == "" || !ok {
				add(n, "Grid row "+preview(line)+" is not | <fact> | <value> | <count> |.", "Use a peer fact as the column and a whole number of peers.")
				continue
			}
			grid = append(grid, GridRow{Line: n, Column: cells[0], Value: cells[1], Peers: count})
		case section == "baseline":
			baseline = append(baseline, strings.TrimSpace(line))
		case section == "gaps" && strings.HasPrefix(line, "- "):
			gaps = append(gaps, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		default:
			add(n, "Unexpected line: "+preview(line), "Peers are a heading plus one \"- Fact: value\" line per fact; then the grid table, the baseline paragraph and the gaps list.")
		}
	}
	flush()
	return c, problems
}

func parsePeerHeading(line string, n int) (Peer, []Problem) {
	format := "### P001 · <name> · https://… · fetched YYYY-MM-DD · via " + strings.Join(Methods, "|")
	fields := strings.Split(strings.TrimPrefix(line, "### "), sep)
	if len(fields) != 5 {
		return Peer{}, []Problem{{n, "Peer heading does not match the format.", "Write it as: " + format}}
	}
	var problems []Problem
	p := Peer{Line: n, Name: strings.TrimSpace(fields[1]), URL: strings.Trim(fields[2], "<>"), Facts: map[string]string{}}
	var ok bool
	if p.ID, ok = entryID(fields[0], "P"); !ok {
		problems = append(problems, Problem{n, "Peer ID " + quote(fields[0]) + " is not P followed by at least three digits.", "Continue the sequence: P001, P002, …"})
	}
	if p.Name == "" {
		problems = append(problems, Problem{n, "Peer has no name.", "Name the product or page."})
	}
	if !webURL(p.URL) {
		problems = append(problems, Problem{n, "Peer URL " + quote(p.URL) + " is not an absolute web URL.", "Link the page you analyzed."})
	}
	if fetched, found := strings.CutPrefix(fields[3], "fetched "); !found {
		problems = append(problems, Problem{n, "Peer heading lacks its fetched date.", "Write it as: " + format})
	} else if p.Fetched, ok = date(fetched); !ok {
		problems = append(problems, Problem{n, "Fetched date " + quote(fetched) + " is not YYYY-MM-DD.", "Use the day you opened the page."})
	}
	if via, found := strings.CutPrefix(fields[4], "via "); !found || !oneOf(via, Methods) {
		problems = append(problems, Problem{n, "Fetch method " + quote(fields[4]) + " is not via " + strings.Join(Methods, ", ") + ".", "Say how the page was obtained."})
	} else {
		p.Via = via
	}
	return p, problems
}

func entryCount(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 1_000_000 {
			return 0, false
		}
	}
	return n, true
}

func quote(s string) string { return "\"" + preview(s) + "\"" }

// preview shortens untrusted text for a message.
func preview(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// Blockquote formats verbatim text as quote lines that parse back to exactly
// the same text: every line gets "> ", empty lines a bare ">".
func Blockquote(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	quoted := strings.Split(text, "\n")
	for i, line := range quoted {
		if line == "" {
			quoted[i] = ">"
		} else {
			quoted[i] = "> " + line
		}
	}
	return strings.Join(quoted, "\n")
}

// mergedFact names another peer fact written into a value, as in
// "navy · Type: sans".
func mergedFact(value string) string {
	for _, field := range PeerFields {
		if strings.Contains(value, field+": ") {
			return field
		}
	}
	return ""
}
