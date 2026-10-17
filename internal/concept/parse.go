// Package concept reads concept.md, the handover from the design phase to
// implementation, and checks it against the research it must stand on.
//
// The file has fixed sections in a fixed order and free content inside them:
// the headings guarantee every decision is made, the words are the author's.
// Decisions cite research by ID (V012 for a voice, P003 for a peer), so every
// claim can be traced back to what real people wrote or what pages show.
package concept

import (
	"regexp"
	"strings"
)

// File is concept.md in the project root.
const File = "concept.md"

// Section names in their required order. Optional ones may be omitted.
var Sections = []struct {
	Name     string
	Optional bool
}{
	{"Diagnosis", true},
	{"Voices", false},
	{"Buyer", false},
	{"Decision path", false},
	{"Mechanism", false},
	{"Offer", false},
	{"Facts", false},
	{"Axes", false},
	{"Sparks", false},
	{"Directions", false},
	{"Concept", false},
	{"Spine", false},
	{"Palette", false},
	{"Type", false},
	{"Form", false},
	{"Imagery", false},
	{"Motion", false},
	{"Copy", false},
	{"Qualities", true},
	{"Changelog", true},
}

// Line is one source line of a section.
type Line struct {
	N    int
	Text string
}

// Section is the content under one "## " heading.
type Section struct {
	Name  string
	Line  int
	Lines []Line // non-empty lines, excluding the heading
}

// Document is a parsed concept.md.
type Document struct {
	Title    string
	Sections map[string]*Section
	Order    []*Section
}

// Problem is a format defect at a line.
type Problem struct {
	Line    int
	Message string
	Hint    string
}

var citation = regexp.MustCompile(`\b[VP][0-9]{3,6}\b`)

// Citations returns the research IDs cited in text, in order of appearance.
func Citations(text string) []string { return citation.FindAllString(text, -1) }

// StripCitations removes research IDs and the spaces they leave.
func StripCitations(text string) string {
	return strings.Join(strings.Fields(citation.ReplaceAllString(text, "")), " ")
}

// Parse splits concept.md into its sections.
func Parse(text string) (Document, []Problem) {
	d := Document{Sections: map[string]*Section{}}
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if title, ok := strings.CutPrefix(lines[0], "# Concept: "); ok && strings.TrimSpace(title) != "" {
		d.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Concept: <project>\".", "Restore the title line.")
	}
	known := map[string]int{}
	for i, s := range Sections {
		known[s.Name] = i
	}
	var current *Section
	last := -1
	for i := 1; i < len(lines); i++ {
		line, n := strings.TrimRight(lines[i], " \t"), i+1
		if name, ok := strings.CutPrefix(line, "## "); ok {
			name = strings.TrimSpace(name)
			index, isKnown := known[name]
			switch {
			case !isKnown:
				add(n, "Unknown section "+quote(name)+".", "Use only the sections of the format, in order; put this content into the closest one.")
				current = nil
				continue
			case d.Sections[name] != nil:
				add(n, "Section "+name+" appears twice.", "Merge both into one section.")
				current = nil
				continue
			case index < last:
				add(n, "Section "+name+" is out of order.", "Keep the sections in the order of the format.")
			}
			last = index
			current = &Section{Name: name, Line: n}
			d.Sections[name] = current
			d.Order = append(d.Order, current)
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if current == nil {
			add(n, "Text outside any section: "+preview(line), "Move it under the section it belongs to.")
			continue
		}
		current.Lines = append(current.Lines, Line{n, line})
	}
	for _, s := range Sections {
		if !s.Optional && d.Sections[s.Name] == nil {
			add(len(lines), "Missing section "+s.Name+".", "Every decision needs its section; write it even if the answer is short.")
		}
	}
	return d, problems
}

// Items returns the list items of a section: lines starting with "- " or
// "1. ", with continuation lines joined. Other lines are ignored.
func (s *Section) Items() []Line {
	if s == nil {
		return nil
	}
	var items []Line
	for _, l := range s.Lines {
		if text, ok := listItem(l.Text); ok {
			items = append(items, Line{l.N, text})
		} else if len(items) > 0 && strings.HasPrefix(l.Text, "  ") {
			items[len(items)-1].Text += " " + strings.TrimSpace(l.Text)
		}
	}
	return items
}

func listItem(line string) (string, bool) {
	if rest, ok := strings.CutPrefix(line, "- "); ok {
		return strings.TrimSpace(rest), true
	}
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	if digits > 0 && strings.HasPrefix(line[digits:], ". ") {
		return strings.TrimSpace(line[digits+2:]), true
	}
	return "", false
}

// Head returns the first line of the item starting at line n, without its
// list marker and continuation lines.
func (s *Section) Head(n int) string {
	if s == nil {
		return ""
	}
	for _, l := range s.Lines {
		if l.N == n {
			text, _ := listItem(l.Text)
			return text
		}
	}
	return ""
}

// Numbered reports whether every item line of the section is numbered.
func (s *Section) Numbered() bool {
	if s == nil {
		return false
	}
	for _, l := range s.Lines {
		if _, ok := listItem(l.Text); ok && strings.HasPrefix(l.Text, "- ") {
			return false
		}
	}
	return true
}

// Text joins the section's lines.
func (s *Section) Text() string {
	if s == nil {
		return ""
	}
	parts := make([]string, len(s.Lines))
	for i, l := range s.Lines {
		parts[i] = l.Text
	}
	return strings.Join(parts, "\n")
}

// Row is one table row with its cells.
type Row struct {
	N     int
	Cells []string
}

// Table returns the data rows of the section's table and whether its header
// matches columns. Separator rows are skipped.
func (s *Section) Table(columns []string) (rows []Row, header bool) {
	if s == nil {
		return nil, false
	}
	for _, l := range s.Lines {
		if !strings.HasPrefix(l.Text, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(l.Text, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if strings.Trim(strings.Join(cells, ""), "-: ") == "" {
			continue
		}
		if !header && len(rows) == 0 && equalFold(cells, columns) {
			header = true
			continue
		}
		rows = append(rows, Row{l.N, cells})
	}
	return rows, header
}

func equalFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Field returns the value of a "Label: value" line in the section.
func (s *Section) Field(label string) (string, int) {
	if s == nil {
		return "", 0
	}
	for _, l := range s.Lines {
		text := strings.TrimPrefix(l.Text, "- ")
		if value, ok := strings.CutPrefix(text, label+": "); ok {
			return strings.TrimSpace(value), l.N
		}
	}
	return "", 0
}

func quote(s string) string { return "\"" + preview(s) + "\"" }

func preview(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}
