package spreadsheet

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/Anonyfox/lfm/internal/formula"
)

// book is an edition filled with one dataset: the inputs as the buyer would
// type them, every formula computed on demand and remembered.
type book struct {
	ed     *edition
	byName map[string]*laid
	inputs map[cellKey]formula.Value
	memo   map[cellKey]formula.Value
	busy   map[cellKey]bool
	cycle  string // the first circular reference met
	// volatile holds the cells whose value depends on TODAY(), directly or
	// through another cell; reading one marks the reader.
	volatile map[cellKey]bool
	reading  *bool
	// unportable holds, per formula source, what it does that the apps
	// compute differently.
	unportable map[*spec]string
	// areas memoizes the ranges formulas read again, such as a 1000-row
	// column every category's SUMIFS scans, and whether one holds a
	// volatile cell. A range is kept from its second read, so ranges read
	// once, as a running total's per row, are not held.
	areas map[areaKey]*areaValue
}

type areaKey struct {
	sheet          string
	c1, r1, c2, r2 int
}

type areaValue struct {
	rows     [][]formula.Value
	cells    *formula.Run // rows flattened, as the *IFS functions read them
	volatile bool
}

type cellKey struct {
	sheet *laid
	pos   Pos
}

// fill builds the book of a dataset. Tables a dataset leaves out keep their
// seed rows; a dataset's table replaces them. Problems name the dataset path.
func fill(ed *edition, w Workbook, name string) (*book, []problem) {
	formulas := 0
	for _, l := range ed.Sheets {
		for _, sp := range l.Cells {
			if sp.Formula != nil {
				formulas++
			}
		}
	}
	b := &book{ed: ed, byName: map[string]*laid{}, inputs: map[cellKey]formula.Value{}, memo: make(map[cellKey]formula.Value, formulas), busy: map[cellKey]bool{}, volatile: map[cellKey]bool{}, unportable: map[*spec]string{}, areas: map[areaKey]*areaValue{}}
	var problems []problem
	for _, l := range ed.Sheets {
		b.byName[l.Name] = l
	}
	data := w.Datasets[name]
	for i, s := range w.Sheets {
		l := ed.Sheets[i]
		switch {
		case l.Table != nil:
			rows := s.Seed
			path := fmt.Sprintf("sheets[%d].seed", i)
			if r, ok := data.Tables[s.Key]; ok {
				rows, path = r, fmt.Sprintf("datasets.%s.tables.%s", name, s.Key)
			}
			if len(rows) > l.Table.Last-l.Table.First+1 {
				problems = append(problems, problem{path, fmt.Sprintf("%d rows do not fit the %d rows of %s.", len(rows), l.Table.Last-l.Table.First+1, s.Key), "Raise rows on the sheet, or shorten the data."})
				rows = rows[:l.Table.Last-l.Table.First+1]
			}
			for r, row := range rows {
				for key, raw := range row {
					c := columnIndex(s.Columns, key)
					if c < 0 {
						problems = append(problems, problem{fmt.Sprintf("%s[%d].%s", path, r, key), fmt.Sprintf("%s has no column %q.", s.Key, key), "Use the column keys of the sheet."})
						continue
					}
					col := s.Columns[c]
					if col.Formula != "" {
						problems = append(problems, problem{fmt.Sprintf("%s[%d].%s", path, r, key), fmt.Sprintf("Column %s is computed; data cannot fill it.", key), "Remove the value; the formula computes it."})
						continue
					}
					v, err := value(ed.word(localized(raw, ed.Language)), col.Type, ed.Language)
					if err != nil {
						problems = append(problems, problem{fmt.Sprintf("%s[%d].%s", path, r, key), err.Error(), "Give dates as 2026-10-01, amounts as numbers."})
						continue
					}
					b.inputs[cellKey{l, Pos{c + 1, l.Table.First + r}}] = v
				}
			}
		default:
			for _, p := range l.Order {
				sp := l.Cells[p]
				if !sp.Input {
					continue
				}
				raw := sp.Default
				if v, ok := data.Cells[sp.Name]; ok && sp.Name != "" {
					raw = v
				}
				if raw == nil {
					continue
				}
				v, err := value(ed.word(localized(raw, ed.Language)), sp.Type, ed.Language)
				if err != nil {
					problems = append(problems, problem{fmt.Sprintf("datasets.%s.cells.%s", name, sp.Name), err.Error(), "Give dates as 2026-10-01, amounts as numbers."})
					continue
				}
				b.inputs[cellKey{l, p}] = v
			}
		}
	}
	for _, cell := range slices.Sorted(maps.Keys(data.Cells)) {
		if t, ok := ed.names[cell]; !ok || t.area != nil {
			problems = append(problems, problem{fmt.Sprintf("datasets.%s.cells.%s", name, cell), fmt.Sprintf("%q names no input cell.", cell), "Fill named cells of forms and reports here; tables go under tables."})
		}
	}
	return b, problems
}

func columnIndex(cols []Column, key string) int {
	for i, c := range cols {
		if c.Key == key {
			return i
		}
	}
	return -1
}

// value turns a JSON value into a cell value of a type: dates from
// 2026-10-01, numbers as they are, localized text in the edition's language.
func value(raw any, typ, lang string) (formula.Value, error) {
	raw = localized(raw, lang)
	switch x := raw.(type) {
	case nil:
		return formula.Value{}, nil
	case float64:
		if typ == "date" {
			return formula.Value{}, fmt.Errorf("%v is a number where a date like 2026-10-01 belongs", x)
		}
		return formula.NumberValue(x), nil
	case bool:
		return formula.BoolValue(x), nil
	case string:
		if typ == "date" {
			t, err := time.Parse("2006-01-02", x)
			if err != nil {
				return formula.Value{}, fmt.Errorf("%q is not a date like 2026-10-01", x)
			}
			return formula.NumberValue(formula.Serial(t)), nil
		}
		if typ == "money" || typ == "number" || typ == "percent" {
			return formula.Value{}, fmt.Errorf("%q is text where a number belongs", x)
		}
		return formula.TextValue(x), nil
	}
	return formula.Value{}, fmt.Errorf("%v cannot fill a cell", raw)
}

// Cell implements formula.Book.
func (b *book) Cell(sheet string, col, row int) formula.Value {
	l := b.byName[sheet]
	if l == nil {
		return formula.ErrorValue(formula.ErrRef)
	}
	return b.at(l, Pos{col, row})
}

// Area reads a range once per book; a reader of a range holding a
// volatile cell becomes volatile itself, as with a single cell. A whole
// table column, as a placeholder names it, is kept from its first read;
// other ranges from their second, so the ranges a running total reads once
// per row are not held.
func (b *book) Area(sheet string, c1, r1, c2, r2 int) ([][]formula.Value, *formula.Run) {
	k := areaKey{sheet, c1, r1, c2, r2}
	got := b.areas[k]
	if got != nil && got.rows != nil {
		if got.volatile && b.reading != nil {
			*b.reading = true
		}
		return got.rows, got.cells
	}
	l := b.byName[sheet]
	column := l != nil && l.Table != nil && r1 == l.Table.First && r2 == l.Table.Last
	if got == nil && !column { // the cells mark a volatile reader themselves
		b.areas[k] = &areaValue{}
		rows, cells := formula.ReadArea(b, sheet, c1, r1, c2, r2)
		return rows, &formula.Run{Values: cells}
	}
	outer := b.reading
	volatile := false
	b.reading = &volatile
	rows, cells := formula.ReadArea(b, sheet, c1, r1, c2, r2)
	b.reading = outer
	if volatile && outer != nil {
		*outer = true
	}
	run := &formula.Run{Values: cells}
	b.areas[k] = &areaValue{rows: rows, cells: run, volatile: volatile}
	return rows, run
}

func (b *book) at(l *laid, p Pos) formula.Value {
	k := cellKey{l, p}
	if v, ok := b.inputs[k]; ok {
		return v
	}
	sp := l.Cells[p]
	if sp == nil {
		return formula.Value{}
	}
	if sp.Formula == nil {
		if sp.Text != "" {
			return formula.TextValue(sp.Text)
		}
		return formula.Value{}
	}
	if v, ok := b.memo[k]; ok {
		if b.volatile[k] && b.reading != nil {
			*b.reading = true
		}
		return v
	}
	if b.busy[k] {
		if b.cycle == "" {
			b.cycle = l.Name + "!" + formula.CellName(p.Col, p.Row)
		}
		return formula.ErrorValue(formula.ErrRef)
	}
	b.busy[k] = true
	outer := b.reading
	volatile := false
	b.reading = &volatile
	v := formula.Eval(sp.Formula, &formula.Context{Book: b, Sheet: l.Name, Row: p.Row, Today: b.ed.today, Volatile: &volatile,
		Unportable: func(why string) {
			if _, ok := b.unportable[sp]; !ok {
				b.unportable[sp] = why
			}
		}})
	b.reading = outer
	if volatile {
		b.volatile[k] = true
		if outer != nil {
			*outer = true
		}
	}
	delete(b.busy, k)
	if v.Kind == formula.Number && (math.IsNaN(v.Num) || math.IsInf(v.Num, 0)) {
		v = formula.ErrorValue(formula.ErrNum)
	}
	b.memo[k] = v
	return v
}
