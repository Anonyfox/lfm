package spreadsheet

import (
	"fmt"
	"html"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/formula"
)

// locale is how an edition shows numbers, money, percents and dates.
type locale struct {
	group, decimal string
	date           func(y, m, d int) string
	symbol         string
	before         bool // the currency symbol goes before the amount
	percentSpace   bool
}

func localeOf(e Edition) locale {
	l := locale{group: ",", decimal: ".", date: func(y, m, d int) string { return fmt.Sprintf("%d/%d/%d", m, d, y) }, before: true}
	switch strings.SplitN(e.Language, "-", 2)[0] {
	case "de", "nl", "da", "it", "es", "pt", "pl":
		l.group, l.decimal, l.percentSpace = ".", ",", true
		l.date = func(y, m, d int) string { return fmt.Sprintf("%02d.%02d.%d", d, m, y) }
	case "fr":
		l.group, l.decimal, l.percentSpace = " ", ",", true
		l.date = func(y, m, d int) string { return fmt.Sprintf("%02d/%02d/%d", d, m, y) }
	}
	if e.Language == "en-GB" || e.Currency == "GBP" {
		l.date = func(y, m, d int) string { return fmt.Sprintf("%02d/%02d/%d", d, m, y) }
	}
	switch e.Currency {
	case "EUR":
		l.symbol, l.before = "€", e.Language == "en"
	case "GBP":
		l.symbol = "£"
	case "CHF":
		l.symbol = "CHF "
	default:
		l.symbol = "$"
	}
	return l
}

// number writes f with digits after the decimal mark and grouping.
func (l locale) number(f float64, digits int) string {
	s := strconv.FormatFloat(math.Abs(f), 'f', digits, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(l.group)
		}
		b.WriteRune(c)
	}
	if frac != "" {
		b.WriteString(l.decimal + frac)
	}
	if f < 0 && math.Abs(f) >= math.Pow(10, -float64(digits))/2 {
		return "-" + b.String()
	}
	return b.String()
}

// show formats a cell value the way the edition's spreadsheet shows it.
func (l locale) show(v formula.Value, typ string) string {
	switch v.Kind {
	case formula.Blank:
		return ""
	case formula.Text, formula.Error:
		return v.Str
	case formula.Logical:
		return v.String()
	case formula.Array:
		return "#VALUE!"
	}
	switch typ {
	case "money":
		if math.Abs(v.Num) < 0.005 {
			return "–" // the money format's zero section
		}
		n := l.number(v.Num, 2)
		if l.before {
			if strings.HasPrefix(n, "-") {
				return "-" + l.symbol + n[1:]
			}
			return l.symbol + n
		}
		return n + " " + strings.TrimSpace(l.symbol)
	case "percent":
		sep := ""
		if l.percentSpace {
			sep = " "
		}
		return l.number(v.Num*100, 0) + sep + "%"
	case "date":
		t := formula.Date(v.Num)
		return l.date(t.Year(), int(t.Month()), t.Day())
	}
	if v.Num == math.Trunc(v.Num) {
		return l.number(v.Num, 0)
	}
	return l.number(v.Num, 2)
}

// visibleRows is how many rows of a table a preview shows: every filled row
// and a few empty ones after them.
func visibleRows(l *laid, b *book) int {
	last := l.Table.First - 1
	for r := l.Table.First; r <= l.Table.Last; r++ {
		for c := range l.Table.Columns {
			if b.at(l, Pos{c + 1, r}).Kind != formula.Blank {
				last = r
				break
			}
		}
	}
	return min(l.Table.Last, last+3)
}

// sheetHTML renders one sheet as a spreadsheet grid: column letters, row
// numbers, the edition's formats and the workbook's styles and highlights.
func sheetHTML(ed *edition, b *book, l *laid, pal Palette, maxRows int) string {
	loc := localeOf(ed.Edition)
	cols, rows := 1, 1
	for _, p := range l.Order {
		cols, rows = max(cols, p.Col+l.Cells[p].Merge), max(rows, p.Row)
	}
	if l.Table != nil {
		rows = visibleRows(l, b)
	}
	if maxRows > 0 {
		rows = min(rows, maxRows)
	}
	var s strings.Builder
	s.WriteString(`<table class="grid"><colgroup><col class="rn">`)
	for c := 1; c <= cols; c++ {
		w := l.Widths[c]
		if w == 0 {
			w = 10
		}
		fmt.Fprintf(&s, `<col style="width:%dpx">`, int(w*7.2+10))
	}
	s.WriteString(`</colgroup><tr class="letters"><th></th>`)
	for c := 1; c <= cols; c++ {
		fmt.Fprintf(&s, "<th>%s</th>", formula.ColumnName(c))
	}
	s.WriteString("</tr>")
	covered := map[Pos]bool{}
	for r := 1; r <= rows; r++ {
		fmt.Fprintf(&s, `<tr><th class="rn">%d</th>`, r)
		for c := 1; c <= cols; c++ {
			p := Pos{c, r}
			if covered[p] {
				continue
			}
			sp := l.Cells[p]
			if sp == nil {
				s.WriteString("<td></td>")
				continue
			}
			v := b.at(l, p)
			class := []string{sp.Style, sp.Type}
			if sp.Text == "" && sp.Formula == nil && !sp.Input {
				class = class[:0]
			}
			// The first rule that holds colors the cell, as Excel ranks them.
			for _, m := range sp.Marks {
				if marked(b, l, p, v, m) {
					class = append(class, "mark-"+m.style)
					break
				}
			}
			span := ""
			if sp.Merge > 0 {
				span = fmt.Sprintf(` colspan="%d"`, sp.Merge+1)
				for k := 1; k <= sp.Merge; k++ {
					covered[Pos{c + k, r}] = true
				}
			}
			fmt.Fprintf(&s, `<td class="%s"%s>%s</td>`, strings.TrimSpace(strings.Join(class, " ")), span, html.EscapeString(loc.show(v, sp.Type)))
		}
		s.WriteString("</tr>")
	}
	s.WriteString("</table>")
	for _, ch := range l.Charts {
		s.WriteString(chartSVG(ed, b, ch, pal, loc))
	}
	return s.String()
}

func marked(b *book, l *laid, p Pos, v formula.Value, m mark) bool {
	if m.cond != nil {
		got := formula.Eval(m.cond, &formula.Context{Book: b, Sheet: l.Name, Row: p.Row, Today: b.ed.today})
		return got.Kind == formula.Logical && got.Num != 0
	}
	if v.Kind != formula.Number {
		return false
	}
	switch m.op {
	case "<":
		return v.Num < m.value
	case ">":
		return v.Num > m.value
	case "<=":
		return v.Num <= m.value
	case ">=":
		return v.Num >= m.value
	case "=":
		return v.Num == m.value
	}
	return v.Num != m.value
}

// series reads a chart's labels and values, leaving out empty and zero
// rows, and each shown value's row in the range, which picks its color.
func series(b *book, ch laidChart) ([]string, []float64, []int) {
	ctx := &formula.Context{Book: b, Today: b.ed.today}
	labels := formula.Eval(ch.Labels, ctx)
	values := formula.Eval(ch.Values, ctx)
	var ls []string
	var vs []float64
	var rows []int
	for i, row := range values.Rows {
		v := row[0]
		if v.Kind != formula.Number || v.Num == 0 || i >= len(labels.Rows) {
			continue
		}
		ls = append(ls, labels.Rows[i][0].String())
		vs = append(vs, v.Num)
		rows = append(rows, i)
	}
	return ls, vs, rows
}

// sliceColor is the color of the slice of a range's row: the accent for
// the first, then chartColors, in the preview and in the file alike.
func sliceColor(pal Palette, row int) string {
	if row == 0 {
		return pal.Accent
	}
	return chartColors[(row-1)%len(chartColors)]
}

// chartColors are the slices of pie and doughnut charts, after the accent.
var chartColors = []string{"#E07A5F", "#81B29A", "#F2CC8F", "#3D405B", "#9C6644", "#6D597A", "#B56576", "#457B9D", "#A8DADC", "#E9C46A"}

func chartSVG(ed *edition, b *book, ch laidChart, pal Palette, loc locale) string {
	ls, vs, rows := series(b, ch)
	w, h := ch.Width, ch.Height
	if w == 0 {
		w = 480
	}
	if h == 0 {
		h = 300
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<figure class="chart" style="width:%dpx"><figcaption>%s</figcaption><svg viewBox="0 0 %d %d" width="%d" height="%d">`, w, html.EscapeString(ch.Title), w, h-30, w, h-30)
	color := func(i int) string { return sliceColor(pal, rows[i]) }
	switch ch.Type {
	case "pie", "doughnut":
		total := 0.0
		for _, v := range vs {
			total += math.Abs(v)
		}
		cx, cy, r := float64(h-30)/2, float64(h-30)/2, float64(h-30)/2-8
		a := -math.Pi / 2
		for i, v := range vs {
			da := 2 * math.Pi * math.Abs(v) / total
			x1, y1 := cx+r*math.Cos(a), cy+r*math.Sin(a)
			x2, y2 := cx+r*math.Cos(a+da), cy+r*math.Sin(a+da)
			large := 0
			if da > math.Pi {
				large = 1
			}
			if len(vs) == 1 {
				fmt.Fprintf(&s, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy, r, color(i))
			} else {
				fmt.Fprintf(&s, `<path d="M%.1f %.1f L%.1f %.1f A%.1f %.1f 0 %d 1 %.1f %.1f Z" fill="%s" stroke="#fff" stroke-width="1.5"/>`, cx, cy, x1, y1, r, r, large, x2, y2, color(i))
			}
			a += da
		}
		if ch.Type == "doughnut" {
			fmt.Fprintf(&s, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#fff"/>`, cx, cy, r*0.55)
		}
		for i, label := range ls {
			y := 14 + i*18
			if y > h-40 {
				break
			}
			fmt.Fprintf(&s, `<rect x="%d" y="%d" width="10" height="10" fill="%s"/><text x="%d" y="%d">%s</text>`, h-10, y-9, color(i), h+6, y, html.EscapeString(label))
		}
	default:
		if len(vs) == 0 {
			break
		}
		top := 0.0
		for _, v := range vs {
			top = math.Max(top, math.Abs(v))
		}
		n := len(vs)
		if ch.Type == "bar" {
			bh := float64(h-40) / float64(n)
			for i, v := range vs {
				y := 4 + float64(i)*bh
				bw := (float64(w) - 150) * math.Abs(v) / top
				fmt.Fprintf(&s, `<text x="0" y="%.1f">%s</text><rect x="110" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, y+bh*0.65, html.EscapeString(truncate(ls[i], 16)), y+bh*0.15, bw, bh*0.7, pal.Accent)
			}
		} else {
			cw := float64(w-20) / float64(n)
			var pts []string
			for i, v := range vs {
				x := 10 + float64(i)*cw
				bhh := float64(h-60) * math.Abs(v) / top
				if ch.Type == "line" {
					pts = append(pts, fmt.Sprintf("%.1f,%.1f", x+cw/2, float64(h-50)-bhh))
				} else {
					fmt.Fprintf(&s, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x+cw*0.15, float64(h-50)-bhh, cw*0.7, bhh, pal.Accent)
				}
				fmt.Fprintf(&s, `<text x="%.1f" y="%d" text-anchor="middle">%s</text>`, x+cw/2, h-36, html.EscapeString(truncate(ls[i], 8)))
			}
			if ch.Type == "line" {
				fmt.Fprintf(&s, `<polyline points="%s" fill="none" stroke="%s" stroke-width="3"/>`, strings.Join(pts, " "), pal.Accent)
			}
		}
	}
	s.WriteString(`</svg></figure>`)
	return s.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// gridCSS styles the grid like a spreadsheet app, in the workbook's palette.
func gridCSS(pal Palette) string {
	marks := ""
	for _, name := range slices.Sorted(maps.Keys(markColors)) {
		c := markColors[name]
		marks += fmt.Sprintf(".mark-%s{color:%s;%s}", name, c[0], map[bool]string{true: "background:" + c[1] + ";", false: ""}[c[1] != ""])
	}
	return fmt.Sprintf(`.grid{border-collapse:collapse;font:13px/1.25 Arial,Helvetica,sans-serif;color:%s;table-layout:fixed;background:#fff}
.grid td,.grid th{border:1px solid #E3E5E8;padding:3px 6px;height:20px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.grid tr.letters th,.grid th.rn{background:#F3F4F6;color:#6B7280;font-weight:400;text-align:center;font-size:11px}
.grid col.rn{width:36px}
.grid td.money,.grid td.number,.grid td.percent,.grid td.date{text-align:right;font-variant-numeric:tabular-nums}
.grid td.title{font-size:20px;font-weight:700;color:%s;height:30px}
.grid td.heading{background:%s;color:%s;font-weight:700}
.grid td.label{font-weight:700}
.grid td.input{background:%s}
.grid td.note{color:%s;font-style:italic}
.grid td.total{font-weight:700;border-top:2px solid %s}
%s
.chart{display:inline-block;margin:18px 24px 0 0;font:13px Arial,sans-serif;vertical-align:top}
.chart figcaption{font-weight:700;margin-bottom:6px}
.chart text{font-size:12px;fill:#374151}
`, pal.Ink, pal.Accent, pal.Heading, pal.HeadingInk, pal.Input, pal.Muted, pal.Accent, marks)
}

// PreviewHTML is the whole edition as one page with a tab bar, for lfm serve:
// every sheet computed with the dataset, shown as the apps show it.
func previewHTML(ed *edition, b *book, pal Palette, title string) []byte {
	var s strings.Builder
	fmt.Fprintf(&s, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>
body{margin:0;background:#F8F9FA;font-family:Arial,sans-serif}
nav{position:sticky;top:0;background:#fff;border-bottom:1px solid #E3E5E8;padding:8px 12px;display:flex;gap:4px;flex-wrap:wrap}
nav a{padding:6px 12px;border-radius:6px 6px 0 0;color:#374151;text-decoration:none;font-size:13px;background:#F3F4F6}
nav a.hidden{opacity:.55}
section{display:none;padding:16px;overflow:auto}
section:target{display:block}
section:last-of-type{display:block}
section:target ~ section:last-of-type{display:none}
%s</style></head><body><nav>`, html.EscapeString(ed.Language), html.EscapeString(title), gridCSS(pal))
	for _, l := range ed.Sheets {
		class := ""
		if l.Hidden {
			class = ` class="hidden"`
		}
		fmt.Fprintf(&s, `<a href="#%s"%s>%s</a>`, l.Key, class, html.EscapeString(l.Name))
	}
	s.WriteString("</nav>")
	// The first sheet shows until a tab is picked; :target shows the picked one.
	for i, l := range ed.Sheets {
		if i == 0 {
			continue
		}
		fmt.Fprintf(&s, `<section id="%s">%s</section>`, l.Key, sheetHTML(ed, b, l, pal, 60))
	}
	fmt.Fprintf(&s, `<section id="%s">%s</section>`, ed.Sheets[0].Key, sheetHTML(ed, b, ed.Sheets[0], pal, 60))
	s.WriteString("</body></html>")
	return []byte(s.String())
}
