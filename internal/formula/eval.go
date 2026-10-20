package formula

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Kind is the type of a value.
type Kind uint8

// Value kinds.
const (
	Blank Kind = iota
	Number
	Text
	Logical
	Error
	Array // a rectangle of values: a range or an array operation's result
)

// Value is the result of a formula or the content of a cell.
type Value struct {
	Kind  Kind
	Num   float64
	Str   string // text, or the error code
	Rows  [][]Value
	cells *Run // an Array's values row by row, shared by every read of the area
}

// Constructors.
func NumberValue(f float64) Value { return Value{Kind: Number, Num: f} }
func TextValue(s string) Value    { return Value{Kind: Text, Str: s} }
func BoolValue(b bool) Value {
	if b {
		return Value{Kind: Logical, Num: 1}
	}
	return Value{Kind: Logical}
}
func ErrorValue(code string) Value { return Value{Kind: Error, Str: code} }

// The error values.
const (
	ErrDiv0  = "#DIV/0!"
	ErrNA    = "#N/A"
	ErrValue = "#VALUE!"
	ErrRef   = "#REF!"
	ErrName  = "#NAME?"
	ErrNum   = "#NUM!"
)

// maxCells caps the values one area or array operation makes, far above any
// table lfm lays out (50000 rows), so a formula over the whole grid is an
// error instead of exhausting memory; maxText is the longest text a cell
// holds in Excel.
const maxCells, maxText = 1 << 20, 32767

// String shows a value the way a test or a finding names it.
func (v Value) String() string {
	switch v.Kind {
	case Number:
		return formatNumber(v.Num)
	case Text, Error:
		return v.Str
	case Logical:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	case Array:
		return "{array}"
	}
	return ""
}

// Book gives the evaluator the cells of a workbook. A cell holding a formula
// returns its evaluated value; the book evaluates it on demand.
type Book interface {
	Cell(sheet string, col, row int) Value
}

// AreaBook is a Book that reads whole areas itself, such as one that
// memoizes them; the rows it returns are never modified.
type AreaBook interface {
	Book
	Area(sheet string, c1, r1, c2, r2 int) (rows [][]Value, cells *Run)
}

// Run is an area's cells in reading order. Its index of texts, which lets
// SUMIFS and the other *IFS functions find the rows equal to a text without
// scanning, is built on first use.
type Run struct {
	Values []Value
	once   sync.Once
	texts  map[string][]int32 // folded text: positions; blanks under ""
}

// positions are the cells equal to the folded text key, in order.
func (r *Run) positions(key string) []int32 {
	r.once.Do(func() {
		r.texts = map[string][]int32{}
		for i, v := range r.Values {
			switch v.Kind {
			case Blank:
				r.texts[""] = append(r.texts[""], int32(i))
			case Text:
				k := fold(v.Str)
				r.texts[k] = append(r.texts[k], int32(i))
			}
		}
	})
	return r.texts[key]
}

// fold is the case-insensitive form texts are compared in.
func fold(s string) string { return strings.ToLower(s) }

// Context is where a formula is evaluated.
type Context struct {
	Book  Book
	Sheet string // the sheet of the cell, for references without a sheet
	Row   int    // the row of the cell, for Here references
	Today time.Time
	// Volatile is set when the formula reads TODAY(): its value changes
	// with the day it is computed.
	Volatile *bool
	// Unportable receives what the formula did that the apps compute
	// differently, such as joining TRUE into text.
	Unportable func(string)
}

// Eval evaluates n.
func Eval(n Node, c *Context) Value {
	switch x := n.(type) {
	case nil:
		return Value{}
	case Num:
		return NumberValue(float64(x))
	case Str:
		return TextValue(string(x))
	case Bool:
		return BoolValue(bool(x))
	case Err:
		return ErrorValue(string(x))
	case Ref:
		row := x.Row
		if x.Here {
			row = c.Row
		}
		return c.Book.Cell(c.sheetOf(x.Sheet), x.Col, row)
	case Area:
		return c.area(x)
	case Unary:
		v := Eval(x.X, c)
		return mapValues(v, func(v Value) Value {
			f, e := toNumber(v)
			if e != "" {
				return ErrorValue(e)
			}
			switch x.Op {
			case "-":
				return NumberValue(-f)
			case "%":
				return NumberValue(f / 100)
			}
			return NumberValue(f)
		})
	case Binary:
		l, r := Eval(x.L, c), Eval(x.R, c)
		if c.Unportable != nil {
			checkPortable(x.Op, l, r, c.Unportable)
		}
		return binary(x.Op, l, r)
	case Call:
		return functions[x.Name].fn(c, x.Args)
	case arrayNode:
		return x.v
	}
	return ErrorValue(ErrValue)
}

func (c *Context) sheetOf(s string) string {
	if s == "" {
		return c.Sheet
	}
	return s
}

func (c *Context) area(a Area) Value {
	sheet := c.sheetOf(a.Sheet)
	r1, r2 := min(a.From.Row, a.To.Row), max(a.From.Row, a.To.Row)
	c1, c2 := min(a.From.Col, a.To.Col), max(a.From.Col, a.To.Col)
	if (r2-r1+1)*(c2-c1+1) > maxCells {
		return ErrorValue(ErrNum)
	}
	if ab, ok := c.Book.(AreaBook); ok {
		rows, cells := ab.Area(sheet, c1, r1, c2, r2)
		return Value{Kind: Array, Rows: rows, cells: cells}
	}
	rows, cells := ReadArea(c.Book, sheet, c1, r1, c2, r2)
	return Value{Kind: Array, Rows: rows, cells: &Run{Values: cells}}
}

// ReadArea reads an area cell by cell, row by row. The rows share one
// backing slice, which it also returns: the area's cells in reading order.
func ReadArea(b Book, sheet string, c1, r1, c2, r2 int) (rows [][]Value, cells []Value) {
	w := c2 - c1 + 1
	cells = make([]Value, 0, (r2-r1+1)*w)
	for r := r1; r <= r2; r++ {
		for col := c1; col <= c2; col++ {
			cells = append(cells, b.Cell(sheet, col, r))
		}
	}
	rows = make([][]Value, r2-r1+1)
	for i := range rows {
		rows[i] = cells[i*w : (i+1)*w : (i+1)*w]
	}
	return rows, cells
}

// mapValues applies f to a scalar, or to every element of an array.
func mapValues(v Value, f func(Value) Value) Value {
	if v.Kind != Array {
		return f(v)
	}
	rows := make([][]Value, len(v.Rows))
	for i, row := range v.Rows {
		rows[i] = make([]Value, len(row))
		for j, x := range row {
			rows[i][j] = f(x)
		}
	}
	return Value{Kind: Array, Rows: rows}
}

// binary applies an operator, element by element when either side is an
// array (as SUMPRODUCT((range="x")*range) needs); a single value pairs with
// every element.
func binary(op string, l, r Value) Value {
	if l.Kind == Array || r.Kind == Array {
		lr, lc := dims(l)
		rr, rc := dims(r)
		rows, cols := max(lr, rr), max(lc, rc)
		if (lr != rows && lr != 1) || (rr != rows && rr != 1) || (lc != cols && lc != 1) || (rc != cols && rc != 1) {
			return ErrorValue(ErrValue)
		}
		if rows*cols > maxCells {
			return ErrorValue(ErrNum)
		}
		out := make([][]Value, rows)
		for i := range rows {
			out[i] = make([]Value, cols)
			for j := range cols {
				out[i][j] = binary(op, at(l, i, j), at(r, i, j))
			}
		}
		return Value{Kind: Array, Rows: out}
	}
	if l.Kind == Error {
		return l
	}
	if r.Kind == Error {
		return r
	}
	switch op {
	case "&":
		return text(toText(l) + toText(r))
	case "=", "<>", "<", ">", "<=", ">=":
		c := compare(l, r)
		var b bool
		switch op {
		case "=":
			b = c == 0
		case "<>":
			b = c != 0
		case "<":
			b = c < 0
		case ">":
			b = c > 0
		case "<=":
			b = c <= 0
		case ">=":
			b = c >= 0
		}
		return BoolValue(b)
	}
	a, e := toNumber(l)
	if e != "" {
		return ErrorValue(e)
	}
	b, e := toNumber(r)
	if e != "" {
		return ErrorValue(e)
	}
	var x float64
	switch op {
	case "+":
		x = a + b
	case "-":
		x = a - b
	case "*":
		x = a * b
	case "/":
		if b == 0 {
			return ErrorValue(ErrDiv0)
		}
		x = a / b
	case "^":
		x = math.Pow(a, b)
	default:
		return ErrorValue(ErrValue)
	}
	return finite(x)
}

// finite is f, or #NUM! where the apps overflow.
func finite(f float64) Value {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ErrorValue(ErrNum)
	}
	return NumberValue(f)
}

// text is s, or #VALUE! when it is longer than a cell holds.
func text(s string) Value {
	if utf8.RuneCountInString(s) > maxText {
		return ErrorValue(ErrValue)
	}
	return TextValue(s)
}

// whole truncates f to an int for a position or count, clamped so huge
// arguments compare as out of range instead of overflowing.
func whole(f float64) int { return int(max(min(f, 1<<40), -1<<40)) }

func dims(v Value) (int, int) {
	if v.Kind != Array || len(v.Rows) == 0 {
		return 1, 1
	}
	return len(v.Rows), len(v.Rows[0])
}

func at(v Value, i, j int) Value {
	if v.Kind != Array {
		return v
	}
	if len(v.Rows) == 1 {
		i = 0
	}
	if len(v.Rows[i]) == 1 {
		j = 0
	}
	return v.Rows[i][j]
}

// toNumber converts as arithmetic does: blank is 0, TRUE is 1, text that
// reads as a number is that number.
func toNumber(v Value) (float64, string) {
	switch v.Kind {
	case Blank:
		return 0, ""
	case Number, Logical:
		return v.Num, ""
	case Text:
		s := strings.TrimSpace(v.Str)
		f, err := strconv.ParseFloat(s, 64)
		// ParseFloat also reads Inf, NaN and hexadecimal, which no app does.
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || strings.ContainsAny(s, "xX") {
			return 0, ErrValue
		}
		return f, ""
	case Error:
		return 0, v.Str
	}
	return 0, ErrValue
}

func toText(v Value) string {
	switch v.Kind {
	case Number:
		return formatNumber(v.Num)
	case Logical:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	case Array:
		if len(v.Rows) > 0 && len(v.Rows[0]) > 0 {
			return toText(v.Rows[0][0])
		}
	}
	return v.Str
}

// formatNumber writes a number as the General format does: up to 15
// significant digits, no trailing zeros.
func formatNumber(f float64) string {
	if f == 0 {
		return "0" // Excel has no negative zero
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatFloat(f, 'f', 0, 64)
	}
	s := strconv.FormatFloat(f, 'g', 15, 64)
	if strings.Contains(s, "e") {
		return s
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func toBool(v Value) (bool, string) {
	switch v.Kind {
	case Blank:
		return false, ""
	case Number, Logical:
		return v.Num != 0, ""
	case Text:
		switch strings.ToUpper(v.Str) {
		case "TRUE":
			return true, ""
		case "FALSE":
			return false, ""
		}
		return false, ErrValue
	case Error:
		return false, v.Str
	}
	return false, ErrValue
}

// compare orders values as comparisons do: numbers before text before
// logical values; text case-insensitively; a blank equals 0, "" and FALSE.
func compare(a, b Value) int {
	if a.Kind == Blank {
		a = blankLike(b)
	}
	if b.Kind == Blank {
		b = blankLike(a)
	}
	rank := func(v Value) int {
		switch v.Kind {
		case Number:
			return 0
		case Text:
			return 1
		case Logical:
			return 2
		}
		return 3
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	switch a.Kind {
	case Text:
		return strings.Compare(strings.ToLower(a.Str), strings.ToLower(b.Str))
	default:
		switch {
		case a.Num < b.Num:
			return -1
		case a.Num > b.Num:
			return 1
		}
		return 0
	}
}

func blankLike(other Value) Value {
	switch other.Kind {
	case Text:
		return TextValue("")
	case Logical:
		return BoolValue(false)
	}
	return NumberValue(0)
}

// each calls f for every value of v, in reading order.
func each(v Value, f func(Value)) {
	if v.Kind != Array {
		f(v)
		return
	}
	for _, row := range v.Rows {
		for _, x := range row {
			f(x)
		}
	}
}

// flat returns the values of v in reading order.
func flat(v Value) []Value {
	if v.Kind != Array {
		return []Value{v}
	}
	if v.cells != nil {
		return v.cells.Values // shared: callers only read
	}
	if len(v.Rows) == 1 {
		return v.Rows[0]
	}
	n := 0
	for _, row := range v.Rows {
		n += len(row)
	}
	out := make([]Value, 0, n)
	for _, row := range v.Rows {
		out = append(out, row...)
	}
	return out
}

// Serial is the date serial of t: days since 30 December 1899, as Excel,
// Sheets and LibreOffice count from March 1900.
func Serial(t time.Time) float64 {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return float64((d.Unix() - epoch.Unix()) / 86400) // Sub saturates after 292 years
}

// Date is the calendar date of a serial.
func Date(serial float64) time.Time {
	return epoch.AddDate(0, 0, int(math.Floor(serial)))
}

var epoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// checkPortable reports operations LibreOffice computes differently from
// Excel and Google Sheets: logical values joined into text become 1 there,
// and a logical compared with a number counts as 1.
func checkPortable(op string, l, r Value, report func(string)) {
	logical := func(v Value) bool { return v.Kind == Logical }
	switch op {
	case "&":
		if logical(l) || logical(r) {
			report("joins TRUE or FALSE into text, which LibreOffice writes as 1 or 0; use IF(cond, \"yes\", \"no\")")
		}
	case "=", "<>", "<", ">", "<=", ">=":
		if logical(l) && r.Kind == Number || logical(r) && l.Kind == Number {
			report("compares TRUE or FALSE with a number, which LibreOffice and Excel order differently; compare with TRUE or use IF")
		}
	}
}
