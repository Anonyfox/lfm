package deck

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/theme"
)

// Sections of the deck concept, in order.
var Sections = []concept.SectionSpec{
	{Name: "Buyer"}, {Name: "Promise"}, {Name: "Decks"}, {Name: "Look"}, {Name: "Changelog", Optional: true},
}

// DeckColumns are the columns of the Decks table.
var DeckColumns = []string{"Deck", "Job"}

// Plan is the parsed concept.
type Plan struct {
	Name, Buyer, Promise string
	Decks                []Planned
	Theme                string // the preset's name
	Look                 theme.Look
	Lines                map[string]int
}

// Planned is one deck of the set, by its file name.
type Planned struct {
	Key, Job string
	Line     int
}

var (
	deckKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

func checkConcept(text string, add func(line int, message, hint string)) Plan {
	plan := Plan{Lines: map[string]int{}}
	d, problems := concept.ParseFormat(text, "# Decks: ", "<product name>", Sections)
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
		add(s.Line, "Buyer has no \"Not for:\" line.", "Say who these decks are not for; it keeps them few and the listing honest.")
	}
	if s := d.Sections["Decks"]; s != nil {
		rows, header := s.Table(DeckColumns)
		if !header {
			add(s.Line, "Decks needs a table with the columns "+strings.Join(DeckColumns, " | ")+".", "Plan every deck as one row: its file name without .md and what the buyer does with it.")
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if len(row.Cells) != len(DeckColumns) {
				add(row.N, fmt.Sprintf("Decks row has %d cells; the table has %d columns.", len(row.Cells), len(DeckColumns)), "Fill Deck and Job.")
				continue
			}
			p := Planned{Key: strings.Trim(strings.TrimSpace(row.Cells[0]), "`"), Job: strings.TrimSpace(row.Cells[1]), Line: row.N}
			switch {
			case !deckKey.MatchString(p.Key):
				add(row.N, fmt.Sprintf("Deck %q is not a file name of lowercase letters, digits and hyphens.", p.Key), "Name it as its Markdown file, such as briefing for en/briefing.md.")
			case seen[p.Key]:
				add(row.N, fmt.Sprintf("Deck %q is planned twice.", p.Key), "Plan each deck once.")
			}
			seen[p.Key] = true
			if len(strings.Fields(p.Job)) < 3 {
				add(row.N, fmt.Sprintf("Deck %q has no job.", p.Key), "Say what the buyer does with this deck: present it, run it, teach it.")
			}
			plan.Decks = append(plan.Decks, p)
		}
	}
	plan.Look = theme.ParseLook(d.Sections["Look"], add)
	plan.Theme = plan.Look.Theme
	return plan
}

// theme is the plan's theme with its accent and fonts.
func (p Plan) theme() theme.Theme { return p.Look.Resolve() }
