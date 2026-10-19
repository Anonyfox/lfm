package formula

import (
	"strconv"
	"strings"
)

// Print writes n in the syntax xlsx files store: English function names
// with the prefixes newer functions need, commas between arguments, a dot as
// the decimal mark and quoted sheet names. Every app shows the formula in
// its own language from this.
func Print(n Node) string { return PrintAt(n, 0) }

// PrintAt prints n for a cell in row, which Here references take.
func PrintAt(n Node, row int) string {
	var b strings.Builder
	p := printer{row: row}
	p.write(&b, n, 0)
	return b.String()
}

type printer struct{ row int }

func (p printer) write(b *strings.Builder, n Node, parent int) {
	write := p.write
	refText := p.refText
	switch x := n.(type) {
	case nil:
	case Num:
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 64))
	case Str:
		b.WriteString(`"` + strings.ReplaceAll(string(x), `"`, `""`) + `"`)
	case Bool:
		if x {
			b.WriteString("TRUE")
		} else {
			b.WriteString("FALSE")
		}
	case Err:
		b.WriteString(string(x))
	case Ref:
		if x.Name != "" {
			b.WriteString(x.Name) // the workbook defines it, so formulas read as words
			return
		}
		b.WriteString(refText(x, true))
	case Area:
		from := x.From
		if from.Sheet == "" {
			from.Sheet = x.Sheet
		}
		b.WriteString(refText(from, true))
		b.WriteString(":")
		b.WriteString(refText(Ref{Col: x.To.Col, Row: x.To.Row, AbsCol: x.To.AbsCol, AbsRow: x.To.AbsRow}, false))
	case Unary:
		if x.Op == "%" {
			write(b, x.X, 6)
			b.WriteString("%")
			return
		}
		b.WriteString(x.Op)
		write(b, x.X, 6)
	case Binary:
		prec := precedence[x.Op]
		if prec < parent {
			b.WriteString("(")
		}
		write(b, x.L, prec)
		b.WriteString(x.Op)
		right := prec + 1
		if x.Op == "^" {
			right = prec
		}
		write(b, x.R, right)
		if prec < parent {
			b.WriteString(")")
		}
	case Call:
		b.WriteString(functions[x.Name].prefix + x.Name + "(")
		for i, a := range x.Args {
			if i > 0 {
				b.WriteString(",")
			}
			write(b, a, 0)
		}
		b.WriteString(")")
	}
}

func (p printer) refText(r Ref, sheet bool) string {
	if r.Here {
		r.Row = p.row
	}
	var b strings.Builder
	if sheet && r.Sheet != "" {
		b.WriteString(QuoteSheet(r.Sheet))
		b.WriteString("!")
	}
	if r.AbsCol {
		b.WriteString("$")
	}
	b.WriteString(ColumnName(r.Col))
	if r.AbsRow {
		b.WriteString("$")
	}
	b.WriteString(strconv.Itoa(r.Row))
	return b.String()
}

// QuoteSheet quotes a sheet name for a reference: 'Mein Budget'!A1.
func QuoteSheet(name string) string {
	plain := name != ""
	for i, c := range name {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			plain = false
		}
	}
	// A name that reads like a cell (AB1) must be quoted too.
	if _, err := parseRef(name); err == nil {
		plain = false
	}
	if plain {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}
