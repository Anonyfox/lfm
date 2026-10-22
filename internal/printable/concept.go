package printable

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
)

// Sections of the printable concept, in order.
var Sections = []concept.SectionSpec{
	{Name: "Buyer"}, {Name: "Promise"}, {Name: "Contents"}, {Name: "Editions"}, {Name: "Look"}, {Name: "Changelog", Optional: true},
}

// ContentColumns are the columns of the Contents table.
var ContentColumns = []string{"Part", "Pages", "Job"}

// Plan is the parsed concept.
type Plan struct {
	Name     string
	Buyer    string
	Promise  string
	Parts    []Part
	Sizes    []string // paper size keys, in order
	Axes     []Axis   // variant axes beside the size, such as Ink: color, ink-saver
	Body     string   // font stacks and accent of the Look
	Headings string
	Accent   string
	Lines    map[string]int // section name -> line, for findings
}

// Part is one row of Contents: a group of pages with one job.
type Part struct {
	Name  string
	Pages int
	Job   string
	Line  int
}

// Axis is one way editions vary besides the paper size.
type Axis struct {
	Name   string
	Values []string
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func checkConcept(text string, add func(line int, message, hint string)) Plan {
	plan := Plan{Lines: map[string]int{}}
	d, problems := concept.ParseFormat(text, "# Printable: ", "<product name>", Sections)
	for _, p := range problems {
		add(p.Line, p.Message, p.Hint)
	}
	plan.Name = strings.TrimSpace(d.Title)
	for name, s := range d.Sections {
		plan.Lines[name] = s.Line
	}
	if s := d.Sections["Buyer"]; s != nil {
		plan.Buyer = sectionText(s)
		if !slices.ContainsFunc(s.Lines, func(l concept.Line) bool {
			return strings.HasPrefix(strings.ToLower(strings.TrimLeft(l.Text, "-* ")), "not for:")
		}) {
			add(s.Line, "Buyer has no \"Not for:\" line.", "Say who this product is not for; it keeps the pages focused and the listing honest.")
		}
	}
	if s := d.Sections["Promise"]; s != nil {
		plan.Promise = sectionText(s)
	}
	if s := d.Sections["Contents"]; s != nil {
		rows, header := s.Table(ContentColumns)
		if !header {
			add(s.Line, "Contents needs a table with the columns "+strings.Join(ContentColumns, " | ")+".", "Plan every part of the product as one row: its name as the pages carry it in data-part, its page count, its job.")
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if len(row.Cells) != len(ContentColumns) {
				add(row.N, fmt.Sprintf("Contents row has %d cells; the table has %d columns.", len(row.Cells), len(ContentColumns)), "Fill Part, Pages and Job.")
				continue
			}
			part := Part{Name: strings.TrimSpace(row.Cells[0]), Job: strings.TrimSpace(row.Cells[2]), Line: row.N}
			n, err := strconv.Atoi(strings.TrimSpace(row.Cells[1]))
			if err != nil || n < 1 {
				add(row.N, fmt.Sprintf("Part %q has no page count (got %q).", part.Name, row.Cells[1]), "Give the exact number of pages per edition; the compiler counts them.")
			}
			part.Pages = n
			switch {
			case part.Name == "":
				add(row.N, "A Contents row has no part name.", "Name the part as its pages carry it in data-part.")
			case seen[strings.ToLower(part.Name)]:
				add(row.N, fmt.Sprintf("Part %q appears twice.", part.Name), "Give each part one row.")
			}
			if len(strings.Fields(part.Job)) < 3 {
				add(row.N, fmt.Sprintf("Part %q has no job.", part.Name), "Say what the buyer does with these pages.")
			}
			seen[strings.ToLower(part.Name)] = true
			plan.Parts = append(plan.Parts, part)
		}
		if header && len(plan.Parts) == 0 {
			add(s.Line, "Contents plans no parts.", "Add a row per part: cover, guide, the pages themselves, keys.")
		}
	}
	if s := d.Sections["Editions"]; s != nil {
		for _, item := range s.Items() {
			key, value, ok := strings.Cut(item.Text, ":")
			if !ok {
				add(item.N, fmt.Sprintf("Editions line %q is not \"Name: value, value\".", item.Text), "Write \"Sizes: a4, letter\" and one line per variant, such as \"Ink: color, ink-saver\".")
				continue
			}
			var values []string
			for _, v := range strings.Split(value, ",") {
				if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
					values = append(values, v)
				}
			}
			if strings.EqualFold(strings.TrimSpace(key), "Sizes") {
				for _, v := range values {
					if _, ok := Sizes[v]; !ok {
						add(item.N, fmt.Sprintf("Size %q is unknown.", v), "Use "+SizeNames()+".")
						continue
					}
					plan.Sizes = append(plan.Sizes, v)
				}
				continue
			}
			if len(values) < 1 {
				add(item.N, fmt.Sprintf("Variant %q has no values.", key), "List the values, such as \"Ink: color, ink-saver\".")
				continue
			}
			plan.Axes = append(plan.Axes, Axis{Name: strings.TrimSpace(key), Values: values})
		}
		if len(plan.Sizes) == 0 {
			add(s.Line, "Editions names no paper size.", "Add \"Sizes: a4, letter\": buyers in Europe print A4, in North America Letter.")
		}
	}
	if s := d.Sections["Look"]; s != nil {
		plan.Body, _ = s.Field("Body")
		plan.Headings, _ = s.Field("Headings")
		plan.Accent, _ = s.Field("Accent")
		if plan.Body == "" || plan.Headings == "" {
			add(s.Line, "Look needs \"Body:\" and \"Headings:\" font stacks.", "Name the typefaces as stacks: Body: <family>, <fallback families>, <generic family>.")
		}
		if !hexColor.MatchString(plan.Accent) {
			add(s.Line, fmt.Sprintf("Look has no valid \"Accent:\" color (got %q).", plan.Accent), "Give the accent color as #rrggbb.")
			plan.Accent = ""
		}
	}
	return plan
}

// sectionText is a section's lines joined, list items and "Not for:" lines left out.
func sectionText(s *concept.Section) string {
	var parts []string
	for _, l := range s.Lines {
		t := strings.TrimSpace(l.Text)
		if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") {
			continue
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, " ")
}
