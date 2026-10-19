package spreadsheet

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validate checks the workbook's structure before anything is laid out and
// reports every defect it finds; it returns whether layout can proceed.
func validate(w Workbook, report func(path, message, hint string)) bool {
	ok := true
	fail := func(path, message, hint string) {
		ok = false
		report(path, message, hint)
	}
	if len(w.Editions) == 0 {
		fail("editions", "The workbook has no editions.", `Add at least one: "editions": [{"key": "en", "language": "en", "currency": "USD"}].`)
	}
	var langs []string
	editions := map[string]bool{}
	for i, e := range w.Editions {
		path := fmt.Sprintf("editions[%d]", i)
		switch {
		case !keyPattern.MatchString(strings.ReplaceAll(e.Key, "-", "_")):
			fail(path+".key", fmt.Sprintf("Edition key %q is not lowercase letters, digits and hyphens.", e.Key), "Use a short key such as en or de; it names the files.")
		case editions[e.Key]:
			fail(path+".key", fmt.Sprintf("Edition %q appears twice.", e.Key), "Give each edition its own key.")
		}
		editions[e.Key] = true
		if e.Language == "" {
			fail(path+".language", "An edition has no language.", "Give the language its texts are picked in, such as de.")
		}
		if _, known := moneyFormats[e.Currency]; !known {
			fail(path+".currency", fmt.Sprintf("Currency %q is not one lfm formats.", e.Currency), "Use USD, EUR, GBP, CHF, CAD or AUD.")
		}
		langs = append(langs, e.Language)
	}
	text := func(path string, t Text, what string) {
		if t.Empty() {
			fail(path, what+" is empty.", "Write it, as one text or one per language.")
			return
		}
		if missing := t.Missing(langs); len(missing) > 0 {
			fail(path, fmt.Sprintf("%s has no %s text.", what, strings.Join(missing, ", ")), "Add the text for every edition's language.")
		}
	}
	keys := map[string]bool{}
	names := map[string]string{}
	if len(w.Sheets) == 0 {
		fail("sheets", "The workbook has no sheets.", "Start with a guide sheet, the inputs, and what they compute.")
	}
	for i, s := range w.Sheets {
		path := fmt.Sprintf("sheets[%d]", i)
		switch {
		case !keyPattern.MatchString(s.Key):
			fail(path+".key", fmt.Sprintf("Sheet key %q is not lowercase letters, digits and underscores.", s.Key), "Formulas name sheets by key, such as [log.amount].")
		case keys[s.Key]:
			fail(path+".key", fmt.Sprintf("Sheet %q appears twice.", s.Key), "Give each sheet its own key.")
		}
		keys[s.Key] = true
		text(path+".name", s.Name, "Sheet "+s.Key+"'s name")
		for _, lang := range langs {
			name := s.Name.In(lang)
			switch {
			case len([]rune(name)) > 31:
				fail(path+".name", fmt.Sprintf("Sheet name %q has %d characters; Excel takes 31.", name, len([]rune(name))), "Shorten it.")
			case sheetName.MatchString(name):
				fail(path+".name", fmt.Sprintf("Sheet name %q holds a character Excel refuses (: \\ / ? * [ ]).", name), "Remove it.")
			case strings.EqualFold(name, "history"):
				fail(path+".name", "Excel reserves the sheet name History.", "Pick another name.")
			case names[lang+"/"+strings.ToLower(name)] != "":
				fail(path+".name", fmt.Sprintf("Sheets %s and %s are both named %q.", names[lang+"/"+strings.ToLower(name)], s.Key, name), "Give every sheet its own name in every language.")
			}
			names[lang+"/"+strings.ToLower(name)] = s.Key
		}
		switch s.Kind {
		case "guide":
			if len(s.Lines) == 0 {
				fail(path+".lines", "A guide has no lines.", "Write what each tab does and the three steps to start.")
			}
			for j, l := range s.Lines {
				if l.Empty() {
					continue // an empty line spaces the guide
				}
				text(fmt.Sprintf("%s.lines[%d]", path, j), l, "A guide line")
			}
		case "form", "report":
			cellNames := map[string]bool{}
			for j, c := range s.Cells {
				cp := fmt.Sprintf("%s.cells[%d]", path, j)
				if c.Type != "" && !types[c.Type] {
					fail(cp+".type", fmt.Sprintf("Cell type %q is unknown.", c.Type), "Use text, number, money, percent, date or enum.")
				}
				if c.Style != "" && !styles[c.Style] {
					fail(cp+".style", fmt.Sprintf("Cell style %q is unknown.", c.Style), "Use title, heading, label, input, output, note or total.")
				}
				if c.Name != "" && !keyPattern.MatchString(c.Name) {
					fail(cp+".name", fmt.Sprintf("Cell name %q is not lowercase letters, digits and underscores.", c.Name), "Formulas name cells as [name].")
				}
				if c.Name != "" && cellNames[c.Name] {
					fail(cp+".name", fmt.Sprintf("Cell name %q appears twice.", c.Name), "Give each named cell its own name.")
				}
				cellNames[c.Name] = true
				if s.Kind == "form" && c.Label.Empty() {
					fail(cp+".label", "A form cell has no label.", "Say what the buyer enters here.")
				}
				if !c.Label.Empty() {
					text(cp+".label", c.Label, "A label")
				}
				if !c.Text.Empty() {
					text(cp+".text", c.Text, "A text")
				}
				if c.Type == "enum" && c.From == "" && len(c.Values.One) == 0 && len(c.Values.ByLang) == 0 {
					fail(cp+".values", "An enum cell has no values and no from.", "List the choices, or name the column they come from.")
				}
				checkMarks(cp+".highlight", c.Highlight, fail)
			}
			for j, ch := range s.Charts {
				if _, ok := chartTypes[ch.Type]; !ok {
					fail(fmt.Sprintf("%s.charts[%d].type", path, j), fmt.Sprintf("Chart type %q is unknown.", ch.Type), "Use pie, doughnut, column, bar or line.")
				}
			}
			if s.Kind == "form" && len(s.Charts) > 0 {
				fail(path+".charts", "Forms hold no charts.", "Put charts on a report sheet.")
			}
		case "table":
			if s.Rows < 1 || s.Rows > 50000 {
				fail(path+".rows", fmt.Sprintf("A table holds %d rows.", s.Rows), "Give the rows the template holds, 1 to 50000; a year of daily entries is about 1000.")
			}
			if len(s.Columns) == 0 {
				fail(path+".columns", "A table has no columns.", "Add the columns buyers fill and those the sheet computes.")
			}
			cols := map[string]bool{}
			for j, c := range s.Columns {
				cp := fmt.Sprintf("%s.columns[%d]", path, j)
				switch {
				case !keyPattern.MatchString(c.Key):
					fail(cp+".key", fmt.Sprintf("Column key %q is not lowercase letters, digits and underscores.", c.Key), "Formulas name columns as [sheet.column].")
				case cols[c.Key]:
					fail(cp+".key", fmt.Sprintf("Column %q appears twice.", c.Key), "Give each column its own key.")
				}
				cols[c.Key] = true
				if !types[c.Type] {
					fail(cp+".type", fmt.Sprintf("Column type %q is unknown.", c.Type), "Use text, number, money, percent, date or enum.")
				}
				text(cp+".title", c.Title, "Column "+c.Key+"'s title")
				if c.Type == "enum" && c.Formula == "" && c.From == "" && len(c.Values.One) == 0 && len(c.Values.ByLang) == 0 {
					fail(cp+".values", "An enum column has no values and no from.", "List the choices, or name the column they come from, such as categories.name.")
				}
				checkMarks(cp+".highlight", c.Highlight, fail)
			}
		default:
			fail(path+".kind", fmt.Sprintf("Sheet kind %q is unknown.", s.Kind), "Use guide, form, table or report.")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(w.Datasets)) {
		if !keyPattern.MatchString(name) {
			fail("datasets."+name, fmt.Sprintf("Dataset name %q is not lowercase letters, digits and underscores.", name), "Use example for the filled file; more datasets test edge cases.")
		}
	}
	for _, name := range slices.Sorted(maps.Keys(w.Expect)) {
		if _, has := w.Datasets[name]; !has && name != Empty {
			fail("expect."+name, fmt.Sprintf("Expectations for %q name no dataset.", name), "Expect values of the datasets the workbook has, or of empty.")
		}
	}
	return ok
}

func checkMarks(path string, hs []Highlight, fail func(path, message, hint string)) {
	for i, h := range hs {
		if !marks[h.Style] {
			fail(fmt.Sprintf("%s[%d].style", path, i), fmt.Sprintf("Highlight style %q is unknown.", h.Style), "Use good, bad, warn or muted.")
		}
	}
}
