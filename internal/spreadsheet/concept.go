package spreadsheet

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
)

// Sections of the spreadsheet concept, in order.
var Sections = []concept.SectionSpec{
	{Name: "Buyer"}, {Name: "Promise"}, {Name: "Tabs"}, {Name: "Look"}, {Name: "Changelog", Optional: true},
}

// TabColumns are the columns of the Tabs table.
var TabColumns = []string{"Tab", "Job"}

// Plan is the parsed concept.
type Plan struct {
	Name, Buyer, Promise string
	Tabs                 []Tab
	Accent               string
	Lines                map[string]int
}

// Tab is one planned sheet, by its key in workbook.json.
type Tab struct {
	Key, Job string
	Line     int
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func checkConcept(text string, add func(line int, message, hint string)) Plan {
	plan := Plan{Lines: map[string]int{}}
	d, problems := concept.ParseFormat(text, "# Spreadsheet: ", "<product name>", Sections)
	for _, p := range problems {
		add(p.Line, p.Message, p.Hint)
	}
	plan.Name = strings.TrimSpace(d.Title)
	for name, s := range d.Sections {
		plan.Lines[name] = s.Line
	}
	section := func(name string) string {
		s := d.Sections[name]
		if s == nil {
			return ""
		}
		var parts []string
		for _, l := range s.Lines {
			if t := strings.TrimSpace(l.Text); !strings.HasPrefix(t, "-") {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, " ")
	}
	plan.Buyer, plan.Promise = section("Buyer"), section("Promise")
	if s := d.Sections["Buyer"]; s != nil && !slices.ContainsFunc(s.Lines, func(l concept.Line) bool {
		return strings.HasPrefix(strings.ToLower(strings.TrimLeft(l.Text, "-* ")), "not for:")
	}) {
		add(s.Line, "Buyer has no \"Not for:\" line.", "Say who this template is not for; it keeps the tabs few and the listing honest.")
	}
	if s := d.Sections["Tabs"]; s != nil {
		rows, header := s.Table(TabColumns)
		if !header {
			add(s.Line, "Tabs needs a table with the columns "+strings.Join(TabColumns, " | ")+".", "Plan every sheet as one row: its key in workbook.json and what the buyer does there.")
		}
		for _, row := range rows {
			if len(row.Cells) != len(TabColumns) {
				add(row.N, fmt.Sprintf("Tabs row has %d cells; the table has %d columns.", len(row.Cells), len(TabColumns)), "Fill Tab and Job.")
				continue
			}
			tab := Tab{Key: strings.Trim(strings.TrimSpace(row.Cells[0]), "`"), Job: strings.TrimSpace(row.Cells[1]), Line: row.N}
			if len(strings.Fields(tab.Job)) < 3 {
				add(row.N, fmt.Sprintf("Tab %q has no job.", tab.Key), "Say what the buyer does on this sheet.")
			}
			plan.Tabs = append(plan.Tabs, tab)
		}
	}
	if s := d.Sections["Look"]; s != nil {
		plan.Accent, _ = s.Field("Accent")
		if !hexColor.MatchString(plan.Accent) {
			add(s.Line, fmt.Sprintf("Look has no valid \"Accent:\" color (got %q).", plan.Accent), "Give the accent as #rrggbb: headers and titles take it, inputs a light tint of it.")
			plan.Accent = ""
		}
	}
	return plan
}
