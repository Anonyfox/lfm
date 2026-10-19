package spreadsheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Workbook is deliverables/spreadsheet/workbook.json: the sheets of the
// product, its editions, the datasets it is checked with and what they must
// compute.
type Workbook struct {
	Editions []Edition                 `json:"editions"`
	Today    string                    `json:"today"` // the date TODAY() is evaluated at in checks and previews
	Words    map[string]Text           `json:"words"` // localized words formulas and choices use as [@key] and "@key"
	Sheets   []Sheet                   `json:"sheets"`
	Datasets map[string]Dataset        `json:"datasets"` // "example" fills the example file; "empty" is the blank template
	Expect   map[string]map[string]any `json:"expect"`   // dataset -> cell -> value
}

// Edition is one language and currency the workbook is built in.
type Edition struct {
	Key      string `json:"key"`      // e.g. en, de
	Language string `json:"language"` // the language its texts are picked in
	Currency string `json:"currency"` // ISO code: USD, EUR, GBP, CHF
}

// Sheet is one tab.
type Sheet struct {
	Key     string    `json:"key"` // stable name for placeholders: [log.amount]
	Name    Text      `json:"name"`
	Kind    string    `json:"kind"` // guide, form, table or report
	Hidden  bool      `json:"hidden"`
	Lines   []Text    `json:"lines"`   // guide: one per row; "# " starts a heading
	Cells   []Cell    `json:"cells"`   // form and report
	Columns []Column  `json:"columns"` // table
	Rows    int       `json:"rows"`    // table: how many data rows the template holds
	Seed    []Row     `json:"seed"`    // table: rows the blank template ships with, such as categories
	Charts  []Chart   `json:"charts"`  // report
	Widths  []float64 `json:"widths"`  // report: column widths in characters, from column A
}

// Column is one column of a table.
type Column struct {
	Key       string      `json:"key"`
	Title     Text        `json:"title"`
	Type      string      `json:"type"` // text, number, money, percent, date, enum, check
	Values    TextList    `json:"values"`
	From      string      `json:"from"`    // enum: the column its choices come from, as sheet.column
	Formula   string      `json:"formula"` // computed per row; [.other] is this row's cell of another column
	Width     float64     `json:"width"`
	Highlight []Highlight `json:"highlight"`
	Note      Text        `json:"note"` // shown as the header's comment
}

// Cell is one cell of a form or report.
type Cell struct {
	At        string      `json:"at"`   // report: A1 position; form cells stack from row 2
	Name      string      `json:"name"` // a name formulas use: [income]
	Label     Text        `json:"label"`
	Text      Text        `json:"text"`
	Value     any         `json:"value"` // an input's default
	Formula   string      `json:"formula"`
	Type      string      `json:"type"`  // text, number, money, percent, date, enum, check
	Style     string      `json:"style"` // title, heading, label, input, output, note, total
	Values    TextList    `json:"values"`
	From      string      `json:"from"`
	Highlight []Highlight `json:"highlight"`
	Note      Text        `json:"note"`
	Merge     int         `json:"merge"` // columns the cell spans to the right
}

// Highlight colors a cell when its condition holds: "<0" or a formula.
type Highlight struct {
	If    string `json:"if"`
	Style string `json:"style"` // good, bad, warn, muted
}

// Chart is a native chart on a report.
type Chart struct {
	Type   string `json:"type"` // pie, doughnut, column, bar, line
	Title  Text   `json:"title"`
	Labels string `json:"labels"` // [sheet.column] or a range placeholder
	Values string `json:"values"`
	At     string `json:"at"`
	Width  int    `json:"width"` // pixels
	Height int    `json:"height"`
}

// Row is one row of a dataset or seed: column key -> value. Text values may
// be localized.
type Row map[string]any

// Dataset fills inputs: tables by sheet key get rows, named cells values.
type Dataset struct {
	Tables map[string][]Row `json:"tables"`
	Cells  map[string]any   `json:"cells"`
}

// Text is a string, or one per language: {"en": "Log", "de": "Buchungen"}.
type Text struct {
	One    string
	ByLang map[string]string
}

// UnmarshalJSON takes a string or an object of strings.
func (t *Text) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte(`"`)) {
		return json.Unmarshal(b, &t.One)
	}
	return json.Unmarshal(b, &t.ByLang)
}

// In picks the text for a language: its own, else English, else any.
func (t Text) In(lang string) string {
	if t.ByLang == nil {
		return t.One
	}
	if s, ok := t.ByLang[lang]; ok {
		return s
	}
	base, _, _ := strings.Cut(lang, "-")
	if s, ok := t.ByLang[base]; ok {
		return s
	}
	if s, ok := t.ByLang["en"]; ok {
		return s
	}
	keys := make([]string, 0, len(t.ByLang))
	for k := range t.ByLang {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return t.ByLang[keys[0]]
	}
	return ""
}

// Empty reports whether the text has nothing in any language.
func (t Text) Empty() bool { return t.One == "" && len(t.ByLang) == 0 }

// Missing names the languages among langs the text has no entry for.
func (t Text) Missing(langs []string) []string {
	if t.ByLang == nil {
		return nil
	}
	var out []string
	for _, l := range langs {
		if _, ok := t.ByLang[l]; !ok {
			out = append(out, l)
		}
	}
	return out
}

// TextList is a list of strings, or one list per language.
type TextList struct {
	One    []string
	ByLang map[string][]string
}

// UnmarshalJSON takes a list or an object of lists.
func (t *TextList) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte(`[`)) {
		return json.Unmarshal(b, &t.One)
	}
	return json.Unmarshal(b, &t.ByLang)
}

// In picks the list for a language.
func (t TextList) In(lang string) []string {
	if t.ByLang == nil {
		return t.One
	}
	if l, ok := t.ByLang[lang]; ok {
		return l
	}
	base, _, _ := strings.Cut(lang, "-")
	if l, ok := t.ByLang[base]; ok {
		return l
	}
	return t.ByLang["en"]
}

// localized picks a dataset or seed value for a language: an object of
// strings is a localized text.
func localized(v any, lang string) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	t := Text{ByLang: map[string]string{}}
	for k, x := range m {
		t.ByLang[k] = fmt.Sprint(x)
	}
	return t.In(lang)
}
