package concept

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/color"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/stock"
)

// Minimums each section must meet. The design skill prints the same numbers.
const (
	MinVoiceNotes = 8
	MinPathSteps  = 3
	MinRejected   = 2
	MinFacts      = 10
	MinAxes       = 5
	MinSparks     = 3
	MaxSparks     = 5
	MinDirections = 8
	MinSpine      = 3
	MinColors     = 3
	MinCopy       = 3
)

// Rule IDs reported by Check.
const (
	RuleFormat   = "concept.format"
	RuleEvidence = "concept.evidence"
	RuleDefaults = "concept.defaults"
	RuleOpen     = "concept.open"
)

// Rules describes the concept checks for the rule catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{
			ID:      RuleFormat,
			Summary: "concept.md has every section of the lfm-design format, each holding a decision",
			Help:    "Every required section must exist in order and hold its decision in the expected shape: cited list items, the axes, palette, type and imagery tables, numbered directions marked chosen or struck, and the motion fields. Implementation reads this file as its only brief, so a missing or malformed section means a decision nobody made. Fix the named line to match the format printed by lfm skills design.",
		},
		{
			ID:      RuleEvidence,
			Summary: "concept.md cites existing research, broadly enough to stand on it",
			Help:    "Every V and P ID in concept.md must exist in the research files, the concept must cite a fair share of the voices across several platforms and peers, and sparks must each rest on two or more voices. A concept that cites a handful of quotes from one source is the builder's taste with footnotes. Cite the voices and peers that actually support each decision, and collect more research first when the files are thin.",
		},
		{
			ID:      RuleOpen,
			Summary: "Questions concept.md leaves open for the owner",
			Help:    "Lists every \"Open: <question>\" in concept.md: facts only the owner can supply, such as the price or the deliverable, and content that must be sourced before building. Implementation must not invent these. Answer each question with the owner or the sources, replace the line with the decision, and compile again.",
		},
		{
			ID:      RuleDefaults,
			Summary: "concept.md avoids the palette, type and imagery defaults of generated pages",
			Help:    "Warns when the declared palette uses stock indigo or violet accents or the cream and terracotta house palette, holds no pair readable as text, when a typeface comes from the generator display roster or one stock sans does everything, and when the page plans no imagery at all. These defaults make a page read as generated before anyone reads a word. Choose from the research instead; if this subject truly calls for one of them, say why in the Source column and set this rule to off for the project.",
		},
	}
}

// Summary describes a concept for the compile report.
type Summary struct {
	Present        bool
	Chosen         string // the chosen direction
	Voices, Peers  int    // distinct IDs cited
	Platforms      int    // distinct platforms among cited voices
	Colors, Faces  int
	DeclaredColors []color.Color
	Families       []string
	Imagery        int
	Open           int // open questions
}

// proseWords never appear in a font family name; their presence means the
// cell holds an explanation instead of a stack.
var proseWords = map[string]bool{"then": true, "or": true, "and": true, "at": true, "with": true, "for": true, "in": true, "as": true, "on": true, "plus": true, "fallback": true, "via": true, "the": true}

// prose returns the first entry of a font stack that reads as prose.
func prose(stack string) string {
	for _, name := range strings.Split(stack, ",") {
		words := strings.Fields(strings.Trim(strings.TrimSpace(name), `"'`))
		if len(words) > 5 {
			return strings.TrimSpace(name)
		}
		for _, w := range words {
			if proseWords[w] {
				return strings.TrimSpace(name)
			}
		}
	}
	return ""
}

// Report is the result of checking concept.md.
type Report struct {
	Diagnostics []diagnostic.Diagnostic
	Summary     Summary
}

var (
	placeholder = regexp.MustCompile(`<[^<>]*…[^<>]*>`)
	hexValue    = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	dated       = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}: \S`)
)

type checker struct {
	add      func(rule string, line int, message, hint string)
	voices   map[string]research.Voice
	peers    map[string]bool
	research bool
}

// Check parses concept.md and checks it against the parsed research, which
// may be nil when the files are absent.
func Check(text string, voices *research.Voices, competitors *research.Competitors) Report {
	var r Report
	c := checker{voices: map[string]research.Voice{}, peers: map[string]bool{}}
	c.add = func(rule string, line int, message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{
			Rule: rule, Severity: diagnostic.Warning,
			Location: diagnostic.Location{File: File, Line: line, Column: 1},
			Message:  message, Hint: hint,
		})
	}
	if voices != nil {
		for _, v := range voices.Entries {
			c.voices[fmt.Sprintf("V%03d", v.ID)] = v
		}
	}
	if competitors != nil {
		for _, p := range competitors.Entries {
			c.peers[fmt.Sprintf("P%03d", p.ID)] = true
		}
	}
	c.research = len(c.voices) > 0 || len(c.peers) > 0
	d, problems := Parse(text)
	for _, p := range problems {
		c.add(RuleFormat, p.Line, p.Message, p.Hint)
	}
	r.Summary = c.sections(d)
	for _, sec := range d.Order {
		for _, l := range sec.Lines {
			if _, question, ok := strings.Cut(l.Text, "Open: "); ok && strings.TrimSpace(question) != "" {
				c.add(RuleOpen, l.N, "Open question: "+preview(question), "Get the answer from the owner or a source, then replace the line with the decision.")
				r.Summary.Open++
			}
		}
	}
	r.Summary.Present = true
	c.evidence(d, &r.Summary, voices)
	c.defaults(d)
	for _, s := range d.Order {
		for _, l := range s.Lines {
			if m := placeholder.FindString(l.Text); m != "" {
				c.add(RuleFormat, l.N, "Template placeholder "+m+" left in the concept.", "Replace it with the decision itself.")
			}
		}
	}
	sort.SliceStable(r.Diagnostics, func(i, j int) bool { return r.Diagnostics[i].Location.Line < r.Diagnostics[j].Location.Line })
	return r
}

// ids returns the distinct research IDs of kind (V or P) cited in text.
func ids(text string, kind byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range Citations(text) {
		if id[0] == kind && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (c checker) need(s *Section, ok bool, message, hint string) {
	if s != nil && !ok {
		c.add(RuleFormat, s.Line, message, hint)
	}
}

// sections checks the shape of each section and fills the summary.
func (c checker) sections(d Document) Summary {
	var sum Summary
	if s := d.Sections["Voices"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinVoiceNotes, fmt.Sprintf("Voices codes %d observations; write at least %d.", len(items), MinVoiceNotes), "Code the voices into phrases, feelings, rituals, self-made aesthetic and complaints, each citing its V IDs.")
		for _, it := range items {
			if len(ids(it.Text, 'V')) == 0 {
				c.add(RuleFormat, it.N, "Observation without a voice ID.", "Cite the V IDs it comes from.")
			}
		}
	}
	if s := d.Sections["Buyer"]; s != nil {
		c.need(s, len(ids(s.Text(), 'V')) > 0, "Buyer cites no voices.", "Describe the buyer from the research and cite the V IDs it rests on.")
	}
	if s := d.Sections["Decision path"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinPathSteps && s.Numbered(), fmt.Sprintf("Decision path has %d numbered steps; write at least %d.", len(items), MinPathSteps), "One numbered step per question the visitor asks, in order.")
		for _, it := range items {
			if !strings.Contains(it.Text, " → ") {
				c.add(RuleFormat, it.N, "Step lacks \" → \" between the visitor's question and the section's job.", "Write: <question> → <what the section does about it>.")
			}
		}
	}
	if s := d.Sections["Mechanism"]; s != nil {
		chosen, _ := s.Field("Chosen")
		rejected := 0
		for _, l := range s.Lines {
			if strings.HasPrefix(strings.TrimPrefix(l.Text, "- "), "Rejected: ") {
				rejected++
			}
		}
		c.need(s, chosen != "", "Mechanism names no \"Chosen:\" mechanism.", "Write \"Chosen: <mechanism and why>\".")
		c.need(s, rejected >= MinRejected, fmt.Sprintf("Mechanism rejects %d alternatives; weigh at least %d.", rejected, MinRejected), "Add \"Rejected: <alternative>: <why>\" lines.")
	}
	if s := d.Sections["Offer"]; s != nil {
		for _, field := range []string{"Product", "Price", "Delivery"} {
			value, _ := s.Field(field)
			c.need(s, value != "", "Offer lacks \""+field+":\".", "State what the buyer pays for, the price and how it arrives; write \""+field+": Open: <question>\" when only the owner can answer.")
		}
	}
	if s := d.Sections["Facts"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinFacts, fmt.Sprintf("Facts lists %d; mine at least %d facts true of this subject.", len(items), MinFacts), "Numbers, timings, materials, vocabulary, rituals, failure modes; each with its source.")
		for _, it := range items {
			if len(Citations(it.Text)) == 0 && !strings.Contains(it.Text, "http") {
				c.add(RuleFormat, it.N, "Fact without a source.", "Cite a V or P ID or link the page it comes from.")
			}
		}
	}
	if s := d.Sections["Axes"]; s != nil {
		rows, header := s.Table([]string{"Axis", "Market", "Voices", "Chosen"})
		c.need(s, header, "Axes needs the table | Axis | Market | Voices | Chosen |.", "One row per design axis the data separates.")
		c.need(s, len(rows) >= MinAxes, fmt.Sprintf("Axes has %d rows; find at least %d.", len(rows), MinAxes), "Use the axes where the market and the voices actually differ or agree.")
		for _, row := range rows {
			switch {
			case len(row.Cells) != 4:
				c.add(RuleFormat, row.N, "Axis row needs four cells.", "| <axis> | <market position, P IDs> | <voice position, V IDs> | <chosen position> |")
			case len(ids(row.Cells[1], 'P')) == 0 || len(ids(row.Cells[2], 'V')) == 0 || row.Cells[3] == "":
				c.add(RuleFormat, row.N, "Axis "+quote(row.Cells[0])+" lacks peer IDs, voice IDs or a chosen position.", "The market cell cites P IDs, the voices cell cites V IDs, the chosen cell decides.")
			}
		}
	}
	spark := map[string]bool{}
	if s := d.Sections["Sparks"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinSparks && len(items) <= MaxSparks, fmt.Sprintf("Sparks lists %d; choose %d to %d.", len(items), MinSparks, MaxSparks), "Sparks are specific, recurring voice details no peer uses.")
		for _, it := range items {
			v := ids(it.Text, 'V')
			if len(v) < 2 {
				c.add(RuleFormat, it.N, "Spark rests on fewer than two voices.", "A spark recurs in the voices; cite at least two V IDs.")
			}
			for _, id := range v {
				spark[id] = true
			}
		}
	}
	if s := d.Sections["Directions"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinDirections && s.Numbered(), fmt.Sprintf("Directions lists %d numbered directions; write at least %d.", len(items), MinDirections), "Write directions fast, push each further, then strike.")
		chosen := 0
		for i, it := range items {
			// the verdict ends the item's first line; continuation lines push the direction further
			body, verdict, ok := strings.Cut(s.Head(it.N), " → ")
			switch {
			case !ok:
				c.add(RuleFormat, it.N, "Direction has no verdict.", "End it with \" → chosen\" or \" → struck: <reason>\".")
			case strings.TrimSpace(verdict) == "chosen":
				chosen++
				sum.Chosen = strings.TrimSpace(body)
				if i < 2 {
					c.add(RuleFormat, it.N, "One of the first two directions is chosen.", "The first two directions are the most probable ones; strike them and choose from the rest.")
				}
			case strings.HasPrefix(verdict, "struck: ") && strings.TrimSpace(verdict[len("struck: "):]) != "":
			default:
				c.add(RuleFormat, it.N, "Verdict "+quote(verdict)+" is neither chosen nor struck with a reason.", "End with \" → chosen\" or \" → struck: <reason>\".")
			}
			if ok && len(ids(body, 'V')) == 0 {
				c.add(RuleFormat, it.N, "Direction cites no voice.", "Build each direction on a spark and cite its V IDs.")
			}
		}
		c.need(s, chosen == 1, fmt.Sprintf("Directions marks %d as chosen; choose exactly one.", chosen), "Implementation builds one concept.")
	}
	if s := d.Sections["Concept"]; s != nil {
		cites := false
		for _, id := range ids(s.Text(), 'V') {
			if spark[id] {
				cites = true
			}
		}
		c.need(s, cites, "Concept cites none of the sparks.", "Say how the chosen direction builds on its sparks, citing their V IDs.")
	}
	if s := d.Sections["Spine"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinSpine && s.Numbered(), fmt.Sprintf("Spine has %d numbered sections; write at least %d.", len(items), MinSpine), "One numbered line per page section, in order: <section>: <its job>.")
		for _, it := range items {
			if name, job, ok := strings.Cut(it.Text, ": "); !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(job) == "" {
				c.add(RuleFormat, it.N, "Spine line is not \"<section>: <job>\".", "Name the section and the job it does for the visitor.")
			}
		}
	}
	if s := d.Sections["Palette"]; s != nil {
		rows, header := s.Table([]string{"Role", "Color", "Source"})
		c.need(s, header, "Palette needs the table | Role | Color | Source |.", "One row per color role.")
		c.need(s, len(rows) >= MinColors, fmt.Sprintf("Palette has %d colors; declare at least %d roles.", len(rows), MinColors), "Ground, text and accent at least; every color the page uses belongs here.")
		for _, row := range rows {
			if len(row.Cells) != 3 || row.Cells[0] == "" || row.Cells[2] == "" || !hexValue.MatchString(row.Cells[1]) {
				c.add(RuleFormat, row.N, "Palette row needs a role, a #rrggbb color and its source.", "| <role> | #rrggbb | <where it comes from> |")
				continue
			}
			col, _ := color.ParseHex(row.Cells[1])
			sum.DeclaredColors = append(sum.DeclaredColors, col)
		}
		sum.Colors = len(sum.DeclaredColors)
	}
	if s := d.Sections["Type"]; s != nil {
		rows, header := s.Table([]string{"Role", "Family", "Source"})
		c.need(s, header, "Type needs the table | Role | Family | Source |.", "One row per typeface role.")
		c.need(s, len(rows) >= 1, "Type declares no typeface.", "Name the family for each role, or a system stack.")
		for _, row := range rows {
			if len(row.Cells) != 3 || row.Cells[0] == "" || row.Cells[1] == "" || row.Cells[2] == "" {
				c.add(RuleFormat, row.N, "Type row needs a role, a family and its source.", "| <role> | <family> | <why this face> |")
				continue
			}
			if bad := prose(row.Cells[1]); bad != "" {
				c.add(RuleFormat, row.N, "Family "+quote(bad)+" is prose, not a font name.", "Write only the stack, e.g. Recursive, system-ui, sans-serif; reasons go in the Source column.")
				continue
			}
			sum.Families = append(sum.Families, row.Cells[1])
		}
		sum.Faces = len(sum.Families)
	}
	if s := d.Sections["Form"]; s != nil {
		c.need(s, len(s.Lines) > 0, "Form is empty.", "Decide shape, corners, edges, elevation and the layout idea.")
	}
	if s := d.Sections["Imagery"]; s != nil {
		rows, header := s.Table([]string{"Image", "Job", "Art direction", "Source"})
		none, _ := s.Field("None")
		c.need(s, header || none != "", "Imagery needs the table | Image | Job | Art direction | Source |.", "Plan every image and its job; write \"None: <reason>\" only for a deliberately imageless page.")
		for _, row := range rows {
			if len(row.Cells) != 4 || row.Cells[0] == "" || row.Cells[1] == "" || row.Cells[2] == "" || row.Cells[3] == "" {
				c.add(RuleFormat, row.N, "Imagery row needs an image, its job, its art direction and how it is made.", "| <image> | <job> | <art direction> | <generated, photographed, drawn, rendered> |")
			}
		}
		sum.Imagery = len(rows)
	}
	if s := d.Sections["Motion"]; s != nil {
		for _, field := range []string{"Moment", "Feedback", "Reduced motion"} {
			value, _ := s.Field(field)
			c.need(s, value != "", "Motion lacks \""+field+":\".", "Decide the one orchestrated moment, the feedback motion and what remains with reduced motion.")
		}
	}
	if s := d.Sections["Copy"]; s != nil {
		items := s.Items()
		c.need(s, len(items) >= MinCopy, fmt.Sprintf("Copy lists %d rules; write at least %d.", len(items), MinCopy), "The buyers' words to use, the tone, the words to avoid; each citing voices.")
		for _, it := range items {
			if len(ids(it.Text, 'V')) == 0 {
				c.add(RuleFormat, it.N, "Copy rule without a voice ID.", "Cite the voices whose words or tone it follows.")
			}
		}
	}
	if s := d.Sections["Changelog"]; s != nil {
		for _, it := range s.Items() {
			if !dated.MatchString(it.Text) {
				c.add(RuleFormat, it.N, "Changelog entry does not start with a date.", "Write: - YYYY-MM-DD: <what changed, which batch, what was kept>.")
			}
		}
	}
	return sum
}

// evidence checks that citations exist and spread across the research.
func (c checker) evidence(d Document, sum *Summary, voices *research.Voices) {
	if !c.research {
		c.add(RuleEvidence, 1, "The concept has no research to stand on.", "Collect research first (lfm skills research); the concept cites it by ID.")
		return
	}
	cited := map[string]bool{}
	platforms := map[string]bool{}
	for _, s := range d.Order {
		for _, l := range s.Lines {
			for _, id := range Citations(l.Text) {
				_, isVoice := c.voices[id]
				if !isVoice && !c.peers[id] {
					c.add(RuleEvidence, l.N, id+" does not exist in the research files.", "Cite only IDs present in research/; never cite from memory.")
					continue
				}
				if cited[id] {
					continue
				}
				cited[id] = true
				if isVoice {
					sum.Voices++
					platforms[strings.ToLower(c.voices[id].Platform)] = true
				} else {
					sum.Peers++
				}
			}
		}
	}
	sum.Platforms = len(platforms)
	totalPlatforms := map[string]bool{}
	for _, v := range c.voices {
		totalPlatforms[strings.ToLower(v.Platform)] = true
	}
	if want := min(20, (len(c.voices)+2)/3); sum.Voices < want {
		c.add(RuleEvidence, 1, fmt.Sprintf("The concept cites %d of %d voices; cite at least %d.", sum.Voices, len(c.voices), want), "Ground every section in the voices that support it; a few quotes cannot represent the buyers.")
	}
	if want := min(3, len(totalPlatforms)); sum.Platforms < want {
		c.add(RuleEvidence, 1, fmt.Sprintf("Cited voices come from %d platforms; the research has %d.", sum.Platforms, len(totalPlatforms)), "Draw on voices from several platforms so one community does not stand for all buyers.")
	}
	if want := min(5, len(c.peers)); sum.Peers < want {
		c.add(RuleEvidence, 1, fmt.Sprintf("The concept cites %d of %d peers; cite at least %d.", sum.Peers, len(c.peers), want), "The axes compare with the market; cite the peers each market position rests on.")
	}
}

// defaults reports generator defaults in the declared visual system.
func (c checker) defaults(d Document) {
	if s := d.Sections["Palette"]; s != nil {
		rows, _ := s.Table([]string{"Role", "Color", "Source"})
		var colors []color.Color
		var cream, terra bool
		for _, row := range rows {
			if len(row.Cells) != 3 {
				continue
			}
			col, ok := color.ParseHex(row.Cells[1])
			if !ok {
				continue
			}
			colors = append(colors, col)
			if n, stocked := stock.Nearest(col, stock.Accents, 0.02); stocked {
				c.add(RuleDefaults, row.N, row.Cells[1]+" is "+n.Name+", the stock generator accent.", "Take the accent from the research: what the buyers photograph, the subject's real materials.")
			}
			_, isCream := stock.Nearest(col, stock.Creams, 0.012)
			_, isTerra := stock.Nearest(col, stock.Terracottas, 0.02)
			cream, terra = cream || isCream, terra || isTerra
		}
		if cream && terra {
			c.add(RuleDefaults, s.Line, "Cream ground with terracotta accent: the assistant house palette.", "Choose colors the research supports, not the palette of the tool that wrote the page.")
		}
		best := 0.0
		for i := range colors {
			for j := i + 1; j < len(colors); j++ {
				best = max(best, color.Contrast(colors[i], colors[j]))
			}
		}
		if len(colors) >= 2 && best < 4.5 {
			c.add(RuleDefaults, s.Line, fmt.Sprintf("No two palette colors reach 4.5:1 contrast (best %.1f:1), so no text color is readable.", best), "Declare a text and a ground color with at least 4.5:1 contrast.")
		}
	}
	if s := d.Sections["Type"]; s != nil {
		rows, _ := s.Table([]string{"Role", "Family", "Source"})
		generic := 0
		for _, row := range rows {
			if len(row.Cells) != 3 {
				continue
			}
			family := strings.Split(row.Cells[1], ",")[0]
			if stock.Display(family) {
				c.add(RuleDefaults, row.N, strings.TrimSpace(family)+" is on the generator display roster.", "Choose a face from the subject's world or the buyers' own artifacts.")
			}
			if stock.GenericSans(family) {
				generic++
			}
		}
		if len(rows) > 0 && generic == len(rows) {
			c.add(RuleDefaults, s.Line, "Every role uses a stock sans.", "Give display a typeface chosen for this subject; a stock sans may stay for body text.")
		}
	}
	if s := d.Sections["Imagery"]; s != nil {
		if none, n := s.Field("None"); none != "" {
			c.add(RuleDefaults, n, "The page plans no imagery.", "A modern page carries its concept in pixels and motion; plan at least the images that prove and explain.")
		}
	}
}
