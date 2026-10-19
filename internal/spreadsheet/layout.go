package spreadsheet

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Anonyfox/lfm/internal/formula"
)

// The types a column or cell can take.
var types = map[string]bool{"text": true, "number": true, "money": true, "percent": true, "date": true, "enum": true}

// Styles of report and form cells.
var styles = map[string]bool{"title": true, "heading": true, "label": true, "input": true, "output": true, "note": true, "total": true}

// Highlight styles.
var marks = map[string]bool{"good": true, "bad": true, "warn": true, "muted": true}

// Pos is a cell position; Col and Row count from 1.
type Pos struct{ Col, Row int }

// spec is what one cell of the laid-out workbook holds.
type spec struct {
	Text    string       // a fixed text: a header, label, guide line
	Input   bool         // the buyer types here; datasets fill it
	Default any          // an input's value in the blank template
	Formula formula.Node // a computed cell
	Source  string       // the formula as written, for findings
	Type    string
	Style   string
	Name    string // a named cell or table column key
	Values  []string
	From    string // a dropdown's range, printed
	Marks   []mark
	Note    string
	Merge   int
	Path    string // where workbook.json defines it, for findings
	Ruled   bool   // a table row's cell: a hairline under it, on screen and on paper
}

type mark struct {
	op    string // "<", ">=" … for a value rule; "" for a formula
	value float64
	cond  formula.Node
	style string
}

// laid is one sheet of an edition, placed on its grid.
type laid struct {
	Key, Name, Kind string
	Hidden          bool
	Cells           map[Pos]*spec
	Order           []Pos // cell positions in reading order
	Widths          map[int]float64
	Freeze          int // rows frozen at the top
	Charts          []laidChart
	Table           *tableShape
}

type tableShape struct {
	Columns []Column
	First   int // first data row
	Last    int // last data row
}

type laidChart struct {
	Type, Title    string
	Labels, Values formula.Area
	At             string
	Width, Height  int
}

// edition is the workbook laid out for one edition.
type edition struct {
	Edition
	Sheets []*laid
	sheets map[string]*laid  // by key
	names  map[string]target // named cells and table columns
	words  map[string]string // localized words, by key
	today  time.Time
	report func(path, message, hint string)
}

// word resolves "@key" values to the edition's word.
func (ed *edition) word(v any) any {
	if s, ok := v.(string); ok {
		if key, ok := strings.CutPrefix(s, "@"); ok {
			if w, known := ed.words[key]; known {
				return w
			}
		}
	}
	return v
}

type target struct {
	sheet string // the sheet's name in this edition
	pos   Pos
	area  *formula.Area // a table column
}

// problem is a defect found while laying out, located by JSON path.
type problem struct {
	path, message, hint string
}

// layout places every sheet of an edition and parses every formula. Defects
// are collected, never fatal: one compile reports them all.
func layout(w Workbook, e Edition) (*edition, []problem) {
	ed := &edition{Edition: e, sheets: map[string]*laid{}, names: map[string]target{}, words: map[string]string{}}
	for k, t := range w.Words {
		ed.words[k] = t.In(e.Language)
	}
	var problems []problem
	report := func(path, message, hint string) { problems = append(problems, problem{path, message, hint}) }
	ed.report = report
	ed.today = time.Now().UTC()
	if w.Today != "" {
		if t, err := time.Parse("2006-01-02", w.Today); err == nil {
			ed.today = t
		} else {
			report("today", fmt.Sprintf("today %q is not a date like 2026-10-19.", w.Today), "Give the date TODAY() means in checks and previews, so the expected values stay fixed.")
		}
	}
	lang := e.Language

	// First pass: names and shapes, so formulas may refer to any sheet.
	for i, s := range w.Sheets {
		path := fmt.Sprintf("sheets[%d]", i)
		size := len(s.Cells) + len(s.Lines)
		if s.Kind == "table" {
			size = len(s.Columns) * (s.Rows + 1)
		}
		l := &laid{Key: s.Key, Name: s.Name.In(lang), Kind: s.Kind, Hidden: s.Hidden, Cells: make(map[Pos]*spec, size), Order: make([]Pos, 0, size), Widths: map[int]float64{}}
		ed.Sheets = append(ed.Sheets, l)
		ed.sheets[s.Key] = l
		switch s.Kind {
		case "table":
			l.Table = &tableShape{Columns: s.Columns, First: 2, Last: 1 + s.Rows}
			l.Freeze = 1
			for c, col := range s.Columns {
				area := formula.Area{Sheet: l.Name, From: formula.Ref{Col: c + 1, Row: 2, AbsCol: true, AbsRow: true}, To: formula.Ref{Col: c + 1, Row: 1 + s.Rows, AbsCol: true, AbsRow: true}}
				ed.names[s.Key+"."+col.Key] = target{sheet: l.Name, area: &area}
			}
		case "form":
			for c, cell := range s.Cells {
				if cell.Name != "" {
					ed.names[cell.Name] = target{sheet: l.Name, pos: Pos{3, c + 3}}
				}
			}
		case "report":
			for c, cell := range s.Cells {
				if cell.Name == "" {
					continue
				}
				p, err := parsePos(cell.At)
				if err != nil {
					report(fmt.Sprintf("%s.cells[%d].at", path, c), err.Error(), "Give the cell's position as A1.")
					continue
				}
				ed.names[cell.Name] = target{sheet: l.Name, pos: p}
			}
		}
	}

	parse := func(path, src string, sheet *laid) formula.Node {
		n, err := formula.Parse(src, ed.resolver(sheet))
		if err != nil {
			report(path, fmt.Sprintf("Formula %s: %s.", src, err), "Write formulas in English with commas, as the file stores them; name columns and cells with placeholders: [log.amount], [.amount] for this row, [income].")
			return nil
		}
		return n
	}
	marksOf := func(path string, hs []Highlight, sheet *laid) []mark {
		var out []mark
		for i, h := range hs {
			m := mark{style: h.Style}
			cond := strings.TrimSpace(h.If)
			if op, v, ok := valueRule(cond); ok {
				m.op, m.value = op, v
			} else if strings.HasPrefix(cond, "=") {
				m.cond = parse(fmt.Sprintf("%s[%d].if", path, i), cond, sheet)
			} else {
				report(fmt.Sprintf("%s[%d].if", path, i), fmt.Sprintf("Highlight condition %q is neither a comparison like <0 nor a formula.", cond), "Write <0, >=100, or a formula such as =[.spent]>[.budget].")
			}
			out = append(out, m)
		}
		return out
	}

	for i, s := range w.Sheets {
		l := ed.Sheets[i]
		path := fmt.Sprintf("sheets[%d]", i)
		switch s.Kind {
		case "guide":
			l.Widths[1] = 100
			for r, line := range s.Lines {
				text := line.In(lang)
				st := "note"
				switch {
				case strings.HasPrefix(text, "# "):
					text, st = strings.TrimPrefix(text, "# "), "title"
				case strings.HasPrefix(text, "## "):
					text, st = strings.TrimPrefix(text, "## "), "heading"
				}
				l.put(Pos{1, r + 1}, &spec{Text: text, Style: st})
			}
		case "form":
			l.Widths[1], l.Widths[2], l.Widths[3], l.Widths[4] = 2, 34, 22, 48
			l.put(Pos{2, 1}, &spec{Text: l.Name, Style: "title"})
			for c, cell := range s.Cells {
				cp := fmt.Sprintf("%s.cells[%d]", path, c)
				row := c + 3
				l.put(Pos{2, row}, &spec{Text: cell.Label.In(lang), Style: "label"})
				sp := ed.cellSpec(cell, cp, l, parse, marksOf)
				sp.Path = cp
				if sp.Formula == nil {
					sp.Input = true
					if sp.Style == "" {
						sp.Style = "input"
					}
				}
				l.put(Pos{3, row}, sp)
				if note := cell.Note.In(lang); note != "" {
					l.put(Pos{4, row}, &spec{Text: note, Style: "note"})
				}
			}
		case "report":
			for c, w := range s.Widths {
				l.Widths[c+1] = w
			}
			for c, cell := range s.Cells {
				cp := fmt.Sprintf("%s.cells[%d]", path, c)
				p, err := parsePos(cell.At)
				if err != nil {
					if cell.Name == "" {
						report(cp+".at", err.Error(), "Give the cell's position as A1.")
					}
					continue
				}
				sp := ed.cellSpec(cell, cp, l, parse, marksOf)
				sp.Path = cp
				if sp.Formula == nil && sp.Text == "" && cell.Value != nil {
					sp.Input = true
					if sp.Style == "" {
						sp.Style = "input"
					}
				}
				if _, dup := l.Cells[p]; dup {
					report(cp+".at", fmt.Sprintf("Two cells sit at %s.", cell.At), "Give each cell its own position.")
				}
				l.put(p, sp)
			}
			for c, ch := range s.Charts {
				cp := fmt.Sprintf("%s.charts[%d]", path, c)
				lc := laidChart{Type: ch.Type, Title: ch.Title.In(lang), At: ch.At, Width: ch.Width, Height: ch.Height}
				for _, part := range []struct {
					src  string
					dest *formula.Area
					name string
				}{{ch.Labels, &lc.Labels, "labels"}, {ch.Values, &lc.Values, "values"}} {
					n, err := formula.Parse(part.src, ed.resolver(l))
					a, ok := n.(formula.Area)
					if err != nil || !ok {
						report(cp+"."+part.name, fmt.Sprintf("Chart %s %q is not a range.", part.name, part.src), "Name a table column, such as [categories.spent].")
						continue
					}
					*part.dest = a
				}
				if _, err := parsePos(ch.At); err != nil {
					report(cp+".at", err.Error(), "Give the chart's top-left cell as A1.")
				}
				l.Charts = append(l.Charts, lc)
			}
		case "table":
			for c, col := range s.Columns {
				cp := fmt.Sprintf("%s.columns[%d]", path, c)
				width := col.Width
				if width == 0 {
					width = map[string]float64{"date": 12, "money": 13, "number": 11, "percent": 9, "enum": 18}[col.Type]
					if width == 0 {
						width = 24
					}
				}
				// A header may wrap between words, never inside one: the column
				// fits the header's longest word in bold.
				for _, word := range strings.Fields(col.Title.In(lang)) {
					width = math.Max(width, float64(utf8.RuneCountInString(word))*1.15+2)
				}
				l.Widths[c+1] = width
				// The header takes its column's type, so it aligns with the figures below.
				l.put(Pos{c + 1, 1}, &spec{Text: col.Title.In(lang), Style: "heading", Type: col.Type, Note: col.Note.In(lang)})
				var node formula.Node
				if col.Formula != "" {
					node = parse(cp+".formula", col.Formula, l)
				}
				values, from := ed.choices(col.Values, col.From, lang, cp, report)
				marks := marksOf(cp+".highlight", col.Highlight, l)
				// Every row of a column shares its spec; specs are never changed
				// after layout.
				sp := &spec{Type: col.Type, Name: col.Key, Values: values, From: from, Marks: marks, Path: cp, Note: col.Note.In(lang), Ruled: true}
				if node != nil {
					sp.Formula, sp.Source, sp.Style = node, col.Formula, "output"
				} else {
					sp.Input, sp.Style = true, "input"
				}
				for r := l.Table.First; r <= l.Table.Last; r++ {
					l.put(Pos{c + 1, r}, sp)
				}
			}
		}
	}
	return ed, problems
}

func (l *laid) put(p Pos, s *spec) {
	if _, ok := l.Cells[p]; !ok {
		l.Order = append(l.Order, p)
	}
	l.Cells[p] = s
}

func (ed *edition) cellSpec(cell Cell, path string, l *laid, parse func(string, string, *laid) formula.Node, marksOf func(string, []Highlight, *laid) []mark) *spec {
	sp := &spec{Type: cell.Type, Style: cell.Style, Name: cell.Name, Text: cell.Text.In(ed.Language), Note: cell.Note.In(ed.Language), Merge: cell.Merge}
	if cell.Formula != "" {
		sp.Formula, sp.Source = parse(path+".formula", cell.Formula, l), cell.Formula
		if sp.Style == "" {
			sp.Style = "output"
		}
	}
	if cell.Value != nil {
		sp.Default = ed.word(localized(cell.Value, ed.Language))
	}
	sp.Values, sp.From = ed.choices(cell.Values, cell.From, ed.Language, path, ed.report)
	sp.Marks = marksOf(path+".highlight", cell.Highlight, l)
	return sp
}

// choices resolves an enum's dropdown: fixed values or another column.
func (ed *edition) choices(values TextList, from, lang, path string, report func(path, message, hint string)) ([]string, string) {
	if from == "" {
		var out []string
		for _, v := range values.In(lang) {
			if key, ok := strings.CutPrefix(v, "@"); ok {
				w, known := ed.words[key]
				if !known {
					report(path+".values", fmt.Sprintf("@%s names no word.", key), "Add it to words, with its text per language.")
				}
				v = w
			}
			out = append(out, v)
		}
		return out, ""
	}
	t, ok := ed.names[from]
	if !ok || t.area == nil {
		report(path+".from", fmt.Sprintf("from %q names no table column.", from), "Name the column the choices come from as sheet.column, such as categories.name.")
		return nil, ""
	}
	return nil, formula.Print(*t.area)
}

// resolver resolves placeholders for formulas on sheet.
func (ed *edition) resolver(sheet *laid) formula.Resolver {
	return func(name string) (formula.Node, error) {
		if key, ok := strings.CutPrefix(name, "@"); ok {
			w, known := ed.words[key]
			if !known {
				return nil, fmt.Errorf("[@%s] names no word; add it to words", key)
			}
			return formula.Str(w), nil
		}
		if col, ok := strings.CutPrefix(name, "."); ok {
			if sheet.Table == nil {
				return nil, fmt.Errorf("[.%s] names a column of this row, but %s is not a table", col, sheet.Key)
			}
			t, ok := ed.names[sheet.Key+"."+col]
			if !ok {
				return nil, fmt.Errorf("[.%s]: %s has no column %s", col, sheet.Key, col)
			}
			return formula.Ref{Col: t.area.From.Col, Here: true}, nil
		}
		t, ok := ed.names[name]
		if !ok {
			return nil, fmt.Errorf("[%s] names no table column (sheet.column) or named cell", name)
		}
		if t.area != nil {
			return *t.area, nil
		}
		ref := formula.Ref{Sheet: t.sheet, Col: t.pos.Col, Row: t.pos.Row, AbsCol: true, AbsRow: true}
		if formula.Definable(name) {
			ref.Name = name // the file defines it: buyers read =income-spent
		}
		return ref, nil
	}
}

func parsePos(s string) (Pos, error) {
	n, err := formula.Parse(s, nil)
	r, ok := n.(formula.Ref)
	if err != nil || !ok || r.Sheet != "" {
		return Pos{}, fmt.Errorf("%q is not a cell position like B4", s)
	}
	return Pos{r.Col, r.Row}, nil
}

// valueRule reads a highlight such as "<0" or ">=100".
func valueRule(s string) (string, float64, bool) {
	for _, op := range []string{">=", "<=", "<>", ">", "<", "="} {
		if rest, ok := strings.CutPrefix(s, op); ok {
			var v float64
			if _, err := fmt.Sscan(strings.TrimSpace(rest), &v); err == nil {
				return op, v, true
			}
		}
	}
	return "", 0, false
}
