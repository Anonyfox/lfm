package spreadsheet

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/formula"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/zipx"
	"github.com/xuri/excelize/v2"
)

// moneyFormats pin each currency's symbol and placement with its locale, so
// a copy opened anywhere shows the edition's money; digit grouping still
// follows the viewer's locale.
var moneyFormats = map[string]string{
	"USD": `[$$-409]#,##0.00`,
	"EUR": `#,##0.00\ [$€-407]`,
	"GBP": `[$£-809]#,##0.00`,
	"CHF": `[$CHF-807]\ #,##0.00`,
	"CAD": `[$$-1009]#,##0.00`,
	"AUD": `[$$-C09]#,##0.00`,
}

// moneyFormat adds the negative and zero sections: a zero shows as a dash,
// so a month without spending reads as empty, not as a column of 0.00.
func moneyFormat(f string) string { return f + ";-" + f + `;"–"` }

// Palette is the look of a workbook: its accent and the tints derived from it.
type Palette struct {
	Accent, Ink, Input, Heading, HeadingInk, Muted, Line string
}

// palette derives the workbook's colors from the concept's accent.
func palette(accent string) Palette {
	if accent == "" {
		accent = "#2F5D50"
	}
	return Palette{Accent: accent, Ink: "#1F2328", Input: mix(accent, 0.90), Heading: accent, HeadingInk: "#FFFFFF", Muted: "#6B7280", Line: mix(accent, 0.75)}
}

// mix blends a #rrggbb color with white: 0 is the color, 1 is white.
func mix(hex string, white float64) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return "#F3F4F6"
	}
	ch := func(shift uint) int {
		c := float64((v >> shift) & 0xff)
		return int(c + (255-c)*white + 0.5)
	}
	return fmt.Sprintf("#%02X%02X%02X", ch(16), ch(8), ch(0))
}

var markColors = map[string][2]string{"good": {"#166534", "#DCFCE7"}, "bad": {"#B91C1C", "#FEE2E2"}, "warn": {"#92400E", "#FEF3C7"}, "muted": {"#6B7280", ""}}

// writeXLSX builds the edition's file with the book's inputs filled in and
// every formula's computed value cached, so previews that do not calculate
// (Quick Look, mail, phones) still show numbers. created is the fixed date
// the file carries, so builds are identical.
func writeXLSX(ed *edition, b *book, pal Palette, created, key string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetDocProps(&excelize.DocProperties{Created: created, Modified: created, Creator: "lfm", LastModifiedBy: "lfm", Language: ed.Language}); err != nil {
		return nil, err
	}
	yes, no := true, false
	if err := f.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &yes}); err != nil {
		return nil, err
	}
	// Cells a buyer types outside the styled ranges look like the rest.
	if err := f.SetDefaultFont("Arial"); err != nil {
		return nil, err
	}
	st := &styler{f: f, pal: pal, money: moneyFormats[ed.Currency], date: dateFormat(ed.Edition), ids: map[string]int{}}
	cached := map[int]map[string]formula.Value{}
	for i, l := range ed.Sheets {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", l.Name); err != nil {
				return nil, err
			}
		} else if _, err := f.NewSheet(l.Name); err != nil {
			return nil, err
		}
		cached[i+1] = map[string]formula.Value{}
		if l.Kind != "table" {
			if err := f.SetSheetView(l.Name, 0, &excelize.ViewOptions{ShowGridLines: &no}); err != nil {
				return nil, err
			}
		}
		for _, c := range slices.Sorted(maps.Keys(l.Widths)) {
			col := formula.ColumnName(c)
			if err := f.SetColWidth(l.Name, col, col, l.Widths[c]); err != nil {
				return nil, fmt.Errorf("%s column %s: %w", l.Name, col, err)
			}
		}
		lastRow := 0
		for _, p := range l.Order {
			sp := l.Cells[p]
			cell := formula.CellName(p.Col, p.Row)
			lastRow = max(lastRow, p.Row)
			switch {
			case sp.Formula != nil:
				v := b.at(l, p)
				if err := f.SetCellFormula(l.Name, cell, formula.PrintAt(sp.Formula, p.Row)); err != nil {
					return nil, err
				}
				cached[i+1][cell] = v
			case sp.Text != "":
				if err := f.SetCellStr(l.Name, cell, sp.Text); err != nil {
					return nil, fmt.Errorf("%s!%s: %w", l.Name, cell, err)
				}
			default:
				if v, ok := b.inputs[cellKey{l, p}]; ok {
					if err := setValue(f, l.Name, cell, v); err != nil {
						return nil, err
					}
				}
			}
			// A table's rows share their column's style, set once below.
			if id := st.id(sp); id > 0 && (l.Table == nil || p.Row < l.Table.First) {
				if err := f.SetCellStyle(l.Name, cell, cell, id); err != nil {
					return nil, err
				}
			}
			if sp.Merge > 0 {
				if err := f.MergeCell(l.Name, cell, formula.CellName(p.Col+sp.Merge, p.Row)); err != nil {
					return nil, fmt.Errorf("%s!%s: %w", l.Name, cell, err)
				}
			}
			if sp.Note != "" && sp.Style == "heading" {
				if err := f.AddComment(l.Name, excelize.Comment{Author: "lfm", Cell: cell, Text: sp.Note}); err != nil {
					return nil, fmt.Errorf("%s!%s: %w", l.Name, cell, err)
				}
			}
		}
		if t := l.Table; t != nil {
			if err := f.SetPanes(l.Name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
				return nil, err
			}
			for c := range t.Columns {
				first := l.Cells[Pos{c + 1, t.First}]
				if id := st.id(first); id > 0 {
					if err := f.SetCellStyle(l.Name, formula.CellName(c+1, t.First), formula.CellName(c+1, t.Last), id); err != nil {
						return nil, err
					}
				}
				ref := fmt.Sprintf("%s%d:%s%d", formula.ColumnName(c+1), t.First, formula.ColumnName(c+1), t.Last)
				if err := st.validate(l.Name, ref, first); err != nil {
					return nil, err
				}
				if err := st.highlight(l.Name, ref, first, formula.CellName(c+1, t.First)); err != nil {
					return nil, err
				}
			}
		} else {
			for _, p := range l.Order {
				sp := l.Cells[p]
				cell := formula.CellName(p.Col, p.Row)
				if err := st.validate(l.Name, cell, sp); err != nil {
					return nil, err
				}
				if err := st.highlight(l.Name, cell, sp, cell); err != nil {
					return nil, err
				}
			}
		}
		for _, ch := range l.Charts {
			if err := addChart(f, l.Name, ch, pal, b); err != nil {
				return nil, err
			}
		}
		if err := pageSetup(f, l, ed.Edition); err != nil {
			return nil, err
		}

		if l.Hidden && i > 0 {
			if err := f.SetSheetVisible(l.Name, false); err != nil {
				return nil, err
			}
		}
	}
	// Named cells are defined names, so formulas read as words.
	for _, name := range sortedNames(ed) {
		t := ed.names[name]
		ref := "'" + strings.ReplaceAll(t.sheet, "'", "''") + "'!$" + formula.ColumnName(t.pos.Col) + "$" + fmt.Sprint(t.pos.Row)
		if err := f.SetDefinedName(&excelize.DefinedName{Name: name, RefersTo: ref}); err != nil {
			return nil, err
		}
	}
	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return cacheValues(buf.Bytes(), cached, key)
}

func setValue(f *excelize.File, sheet, cell string, v formula.Value) error {
	switch v.Kind {
	case formula.Number:
		return f.SetCellFloat(sheet, cell, v.Num, -1, 64)
	case formula.Text:
		return f.SetCellStr(sheet, cell, v.Str)
	case formula.Logical:
		return f.SetCellBool(sheet, cell, v.Num != 0)
	}
	return nil
}

// styler makes one excelize style per combination of type and style.
type styler struct {
	f     *excelize.File
	pal   Palette
	money string
	date  string
	ids   map[string]int
}

// dateFormat is how an edition writes dates: as its readers do, not as the
// opening computer's short date (which turns 10/1/2026 into 10/1/26).
func dateFormat(e Edition) string {
	lang, _, _ := strings.Cut(strings.ToLower(e.Language), "-")
	switch {
	case lang == "de", lang == "da", lang == "fi", lang == "nb", lang == "pl", lang == "cs", lang == "ru", lang == "tr":
		return "dd.mm.yyyy"
	case lang == "en" && (e.Currency == "USD" || e.Currency == "CAD"):
		return "m/d/yyyy"
	case lang == "nl":
		return "dd-mm-yyyy"
	case lang == "sv", lang == "ja", lang == "zh", lang == "ko":
		return "yyyy-mm-dd"
	}
	return "dd/mm/yyyy"
}

// paperSize is the edition's paper in Excel's numbering: US Letter where
// the dollar is, A4 elsewhere.
func paperSize(e Edition) int {
	if e.Currency == "USD" || e.Currency == "CAD" {
		return 1
	}
	return 9
}

func (s *styler) id(sp *spec) int {
	key := fmt.Sprintf("%s/%s/%v", sp.Type, sp.Style, sp.Ruled)
	if id, ok := s.ids[key]; ok {
		return id
	}
	style := &excelize.Style{Font: &excelize.Font{Family: "Arial", Size: 10, Color: s.pal.Ink}, Alignment: &excelize.Alignment{Vertical: "center"}}
	switch sp.Type {
	case "money":
		f := s.money
		if f == "" {
			f = "#,##0.00"
		}
		f = moneyFormat(f)
		style.CustomNumFmt = &f
	case "percent":
		style.NumFmt = 9
	case "date":
		style.CustomNumFmt = &s.date
	}
	fill := func(c string) excelize.Fill { return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c}} }
	switch sp.Style {
	case "title":
		style.Font.Size, style.Font.Bold, style.Font.Color = 16, true, s.pal.Accent
	case "heading":
		style.Font.Bold, style.Font.Color = true, s.pal.HeadingInk
		style.Fill = fill(s.pal.Heading)
		style.Alignment.WrapText = true
		switch sp.Type {
		case "money", "number", "percent", "date":
			style.Alignment.Horizontal = "right"
		}
	case "label":
		style.Font.Bold = true
	case "input":
		style.Fill = fill(s.pal.Input)
	case "note":
		style.Font.Color, style.Font.Italic = s.pal.Muted, true
		style.Alignment.WrapText = true
	case "total":
		style.Font.Bold = true
		style.Border = []excelize.Border{{Type: "top", Color: s.pal.Accent, Style: 2}}
	case "output":
	}
	// Table rows get a hairline: Excel prints no gridlines through filled
	// cells, and every app shows a border.
	if sp.Ruled && style.Border == nil {
		style.Border = []excelize.Border{{Type: "bottom", Color: mix(s.pal.Accent, 0.82), Style: 1}}
	}
	id, err := s.f.NewStyle(style)
	if err != nil {
		return 0
	}
	s.ids[key] = id
	return id
}

// validate adds an enum's dropdown. It warns rather than refuses, so buyers
// can type a category of their own: refused own categories are a top
// complaint.
func (s *styler) validate(sheet, ref string, sp *spec) error {
	if sp == nil || !sp.Input {
		return nil
	}
	list := sp.Type == "enum" && (len(sp.Values) > 0 || sp.From != "")
	if !list && sp.Note == "" {
		return nil
	}
	dv := excelize.NewDataValidation(true)
	dv.Sqref = ref
	// A note shows when the buyer selects the cell, in Excel and LibreOffice.
	if sp.Note != "" {
		dv.SetInput("", sp.Note)
	}
	switch {
	case !list:
		return s.f.AddDataValidation(sheet, dv)
	case sp.From != "":
		dv.SetSqrefDropList(sp.From)
	default:
		if err := dv.SetDropList(sp.Values); err != nil {
			return fmt.Errorf("%s %s: %w", sheet, ref, err)
		}
	}
	dv.SetError(excelize.DataValidationErrorStyleWarning, "", "")
	return s.f.AddDataValidation(sheet, dv)
}

// highlight adds a cell's conditional colors; first is the range's top-left
// cell, which a formula rule is written relative to.
func (s *styler) highlight(sheet, ref string, sp *spec, first string) error {
	if sp == nil || len(sp.Marks) == 0 {
		return nil
	}
	var opts []excelize.ConditionalFormatOptions
	for _, m := range sp.Marks {
		c := markColors[m.style]
		style := &excelize.Style{Font: &excelize.Font{Color: c[0]}}
		if c[1] != "" {
			style.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c[1]}}
		}
		id, err := s.f.NewConditionalStyle(style)
		if err != nil {
			return err
		}
		row, _ := strconv.Atoi(strings.TrimLeft(first, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
		if m.cond != nil {
			opts = append(opts, excelize.ConditionalFormatOptions{Type: "formula", Format: &id, Criteria: "=", Value: formula.PrintAt(m.cond, row)})
			continue
		}
		criteria := map[string]string{"<": "<", ">": ">", "<=": "<=", ">=": ">=", "=": "==", "<>": "!="}[m.op]
		opts = append(opts, excelize.ConditionalFormatOptions{Type: "cell", Format: &id, Criteria: criteria, Value: strconv.FormatFloat(m.value, 'f', -1, 64)})
	}
	return s.f.SetConditionalFormat(sheet, ref, opts)
}

var chartTypes = map[string]excelize.ChartType{"pie": excelize.Pie, "doughnut": excelize.Doughnut, "column": excelize.Col, "bar": excelize.Bar, "line": excelize.Line}

// pageSetup prints a sheet as buyers expect: one page wide on the
// edition's paper, wide sheets in landscape, a table's header row on every
// page and only the rows it holds data in.
func pageSetup(f *excelize.File, l *laid, e Edition) error {
	fit, size, zero, one := true, paperSize(e), 0, 1
	orientation := "portrait"
	width := 0.0
	for c := range l.Widths {
		width += l.Widths[c]
	}
	if width > 110 {
		orientation = "landscape"
	}
	if err := f.SetSheetProps(l.Name, &excelize.SheetPropsOptions{FitToPage: &fit}); err != nil {
		return err
	}
	if err := f.SetPageLayout(l.Name, &excelize.PageLayoutOptions{Size: &size, Orientation: &orientation, FitToWidth: &one, FitToHeight: &zero}); err != nil {
		return err
	}
	// The sheet's name and the page on every printed page.
	if err := f.SetHeaderFooter(l.Name, &excelize.HeaderFooterOptions{OddFooter: "&L&8&A&R&8&P / &N"}); err != nil {
		return err
	}
	if t := l.Table; t != nil {
		sheet := "'" + strings.ReplaceAll(l.Name, "'", "''") + "'"
		if err := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Titles", RefersTo: sheet + "!$1:$1", Scope: l.Name}); err != nil {
			return err
		}
		// The rows a buyer has filled, by the first column: styled empty
		// rows would print as blank pages.
		area := fmt.Sprintf("OFFSET(%[1]s!$A$1,0,0,MAX(1,COUNTA(%[1]s!$A$1:$A$%[2]d)),%[3]d)", sheet, t.Last, len(t.Columns))
		if err := f.SetDefinedName(&excelize.DefinedName{Name: "_xlnm.Print_Area", RefersTo: area, Scope: l.Name}); err != nil {
			return err
		}
	}
	return nil
}

func addChart(f *excelize.File, sheet string, ch laidChart, pal Palette, b *book) error {
	w, h := ch.Width, ch.Height
	if w == 0 {
		w = 480
	}
	labels, values := ch.Labels, ch.Values
	// A chart of a table column shows the rows in use and a few spare;
	// Excel keeps a slot for every row of the range, so 40 rows would crush
	// a dozen bars.
	if l := b.byName[labels.Sheet]; l != nil && l.Table != nil {
		used := 0
		for r := l.Table.First; r <= l.Table.Last; r++ {
			if v := b.at(l, Pos{labels.From.Col, r}); v.Kind != formula.Blank && v.String() != "" {
				used = r - l.Table.First + 1
			}
		}
		last := min(l.Table.Last, l.Table.First+used+4)
		labels.To.Row, values.To.Row = last, last
	}
	rows := labels.To.Row - labels.From.Row + 1
	if h == 0 {
		h = 300
		if ch.Type == "bar" {
			h = max(260, 70+20*rows)
		}
	}
	small := &excelize.Font{Size: 9, Color: pal.Muted}
	c := &excelize.Chart{
		Type:      chartTypes[ch.Type],
		Series:    []excelize.ChartSeries{{Name: ch.Title, Categories: formula.Print(labels), Values: formula.Print(values)}},
		Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: ch.Title, Font: &excelize.Font{Bold: true, Size: 11, Color: pal.Ink}}}},
		Dimension: excelize.ChartDimension{Width: uint(w), Height: uint(h)},
		Legend:    excelize.ChartLegend{Position: "right", Font: small},
		XAxis:     excelize.ChartAxis{Font: *small},
		YAxis:     excelize.ChartAxis{Font: *small, MajorGridLines: true},
		Border:    excelize.LineOptions{Type: excelize.LineNone},
	}
	switch ch.Type {
	case "column", "bar", "line":
		c.Legend.Position = "none"
		c.Series[0].Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{pal.Accent}}
		gap := uint(60)
		c.GapWidth = &gap
		if ch.Type == "bar" {
			c.XAxis.ReverseOrder = true // the first row on top, as the table reads
		}
	default:
		if ch.Type == "doughnut" {
			c.HoleSize = 55 // the preview's ring, not Excel's thin default
		}
		// Each slice in its row's color, as the preview shows it.
		for i := range rows {
			c.Series[0].DataPoint = append(c.Series[0].DataPoint, excelize.ChartDataPoint{Index: i, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{sliceColor(pal, i)}}})
		}
	}
	return f.AddChart(sheet, ch.At, c)
}

var defaultType = regexp.MustCompile(`<Default [^>]*></Default>|<Default [^>]*/>`)

// sortDefaults orders the content types excelize writes from a map, so the
// file is the same on every build.
func sortDefaults(xml []byte) []byte {
	found := defaultType.FindAll(xml, -1)
	if len(found) < 2 {
		return xml
	}
	sorted := make([][]byte, len(found))
	copy(sorted, found)
	slices.SortFunc(sorted, bytes.Compare)
	first := defaultType.FindIndex(xml)
	rest := defaultType.ReplaceAll(xml[first[0]:], nil)
	return append(append(append([]byte{}, xml[:first[0]]...), bytes.Join(sorted, nil)...), rest...)
}

// cacheValues writes the computed value of every formula cell into the
// sheet XML excelize wrote, typed as the value is.
func cacheValues(xlsx []byte, cached map[int]map[string]formula.Value, key string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(xlsx) + len(xlsx)/4)
	w := zipx.New(&out, stampTime)
	for _, file := range r.File {
		var index int
		_, notSheet := fmt.Sscanf(file.Name, "xl/worksheets/sheet%d.xml", &index)
		// excelize writes fonts and chart series out of schema order, which
		// Excel refuses; those parts are put in order.
		ordered := file.Name == "xl/styles.xml" || strings.HasPrefix(file.Name, "xl/charts/")
		if file.Name != "[Content_Types].xml" && !ordered && (notSheet != nil || len(cached[index]) == 0) {
			// Unchanged parts keep their compressed bytes.
			if err := w.Copy(file); err != nil {
				return nil, err
			}
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		switch {
		case ordered:
			data = ooxml.Order(data)
		case notSheet != nil:
			data = sortDefaults(data)
		default:
			data = withValues(data, cached[index])
		}
		if err := w.Deflate(file.Name, data); err != nil {
			return nil, err
		}
	}
	if key != "" {
		if err := w.Comment(zipx.StampPrefix + key); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Stamp is the input key a built .xlsx carries in its zip comment.
func Stamp(tail []byte) string { return zipx.Stamp(tail) }

// stampTime is the date every part of a built .xlsx carries, so the same
// workbook gives the same bytes.
var stampTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// withValues rewrites every formula cell excelize wrote,
// <c r="B4" s="3" t="str"><f>…</f></c>, with its value and type.
func withValues(data []byte, values map[string]formula.Value) []byte {
	out := make([]byte, 0, len(data)+len(values)*16)
	for {
		at := bytes.Index(data, cellOpen)
		if at < 0 {
			return append(out, data...)
		}
		out = append(out, data[:at]...)
		data = data[at:]
		ref, attr, f, rest, ok := formulaCellAt(data)
		v, known := values[string(ref)]
		if !ok || !known {
			out = append(out, cellOpen...)
			data = data[len(cellOpen):]
			continue
		}
		out = append(out, `<c r="`...)
		out = append(out, ref...)
		out = append(out, '"')
		out = append(out, attr...)
		typ, text := "str", v.Str
		switch v.Kind {
		case formula.Number:
			typ, text = "", strconv.FormatFloat(v.Num, 'g', 17, 64)
		case formula.Logical:
			typ, text = "b", "0"
			if v.Num != 0 {
				text = "1"
			}
		case formula.Error:
			typ = "e"
		case formula.Blank:
			text = ""
		default:
			text = html.EscapeString(v.Str)
		}
		if typ != "" {
			out = append(out, ` t="`...)
			out = append(out, typ...)
			out = append(out, '"')
		}
		out = append(out, "><f>"...)
		out = append(out, f...)
		out = append(out, "</f><v>"...)
		out = append(out, text...)
		out = append(out, "</v></c>"...)
		data = rest
	}
}

var cellOpen = []byte(`<c r="`)

// formulaCellAt parses a formula cell as excelize writes it at the start of
// data: the reference, the style attribute, the formula and what follows.
func formulaCellAt(data []byte) (ref, attr, f, rest []byte, ok bool) {
	data = data[len(cellOpen):]
	end := bytes.IndexByte(data, '"')
	if end < 1 {
		return nil, nil, nil, nil, false
	}
	ref, data = data[:end], data[end+1:]
	if bytes.HasPrefix(data, []byte(` s="`)) {
		q := bytes.IndexByte(data[4:], '"')
		if q < 0 {
			return nil, nil, nil, nil, false
		}
		attr, data = data[:4+q+1], data[4+q+1:]
	}
	const open = ` t="str"><f>`
	if !bytes.HasPrefix(data, []byte(open)) {
		return nil, nil, nil, nil, false
	}
	data = data[len(open):]
	end = bytes.Index(data, []byte("</f>"))
	if end < 0 {
		return nil, nil, nil, nil, false
	}
	f, data = data[:end], data[end+4:]
	if bytes.HasPrefix(data, []byte("<v>")) {
		end = bytes.Index(data, []byte("</v>"))
		if end < 0 {
			return nil, nil, nil, nil, false
		}
		data = data[end+4:]
	}
	if !bytes.HasPrefix(data, []byte("</c>")) {
		return nil, nil, nil, nil, false
	}
	return ref, attr, f, data[4:], true
}

// sortedNames are an edition's named cells that become defined names.
func sortedNames(ed *edition) []string {
	var out []string
	for name, t := range ed.names {
		if t.area == nil && formula.Definable(name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}
