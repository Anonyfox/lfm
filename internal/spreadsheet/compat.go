package spreadsheet

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/formula"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/xuri/excelize/v2"
)

// Office finds LibreOffice; see office.Find.
func Office() (string, error) { return office.Find() }

// Recalculate opens every file in LibreOffice, recalculates it and returns
// each sheet's cells as LibreOffice computes them: file name -> sheet name
// -> rows of raw values. One soffice run serves all files.
func Recalculate(ctx context.Context, soffice string, files map[string][]byte) (map[string]map[string][][]string, error) {
	written, err := office.Convert(ctx, soffice, files, "csv:Text - txt - csv (StarCalc):44,34,UTF8,1,,0,false,false,false,false,false,-1")
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][][]string{}
	for name := range files {
		stem := strings.TrimSuffix(name, ".xlsx")
		out[name] = map[string][][]string{}
		for file, data := range written {
			sheet, ok := strings.CutPrefix(file, stem+"-")
			if !ok || !strings.HasSuffix(sheet, ".csv") {
				continue
			}
			r := csv.NewReader(bytes.NewReader(data))
			r.FieldsPerRecord = -1
			rows, err := r.ReadAll()
			if err != nil {
				return nil, err
			}
			out[name][strings.TrimSuffix(sheet, ".csv")] = rows
		}
	}
	return out, nil
}

// compare holds LibreOffice's values to lfm's for every formula cell of a
// book, except those that read TODAY(), and returns the differences.
func compare(ed *edition, b *book, sheets map[string][][]string, app string) []string {
	var out []string
	for _, l := range ed.Sheets {
		rows, ok := sheets[l.Name]
		if !ok {
			out = append(out, fmt.Sprintf("%s returned no sheet %q.", app, l.Name))
			continue
		}
		for _, p := range l.Order {
			sp := l.Cells[p]
			if sp.Formula == nil {
				continue
			}
			want := b.at(l, p)
			if b.volatile[cellKey{l, p}] {
				continue
			}
			got := ""
			if p.Row-1 < len(rows) && p.Col-1 < len(rows[p.Row-1]) {
				got = rows[p.Row-1][p.Col-1]
			}
			if !agrees(got, want, sp.Type) {
				out = append(out, fmt.Sprintf("%s!%s: %s computes %q, lfm %s (%s)", l.Name, formula.CellName(p.Col, p.Row), app, got, want, sp.Source))
				if len(out) >= 40 {
					return out
				}
			}
		}
	}
	return out
}

// agrees reads LibreOffice's raw export of a cell: numbers plain, percents
// with %, dates as month/day/year.
func agrees(got string, want formula.Value, typ string) bool {
	switch want.Kind {
	case formula.Blank:
		return got == ""
	case formula.Text:
		return got == want.Str
	case formula.Error:
		return strings.HasPrefix(got, "#") || got == "Err:502" || strings.HasPrefix(got, "Err:")
	case formula.Logical:
		return strings.EqualFold(got, want.String()) || got == map[bool]string{true: "1", false: "0"}[want.Num != 0]
	}
	f, ok := number(got, typ)
	return ok && math.Abs(f-want.Num) <= 1e-6*math.Max(1, math.Abs(want.Num))
}

func number(s, typ string) (float64, bool) {
	s = strings.TrimSpace(s)
	if p, ok := strings.CutSuffix(s, "%"); ok {
		f, err := strconv.ParseFloat(p, 64)
		return f / 100, err == nil
	}
	if typ == "date" {
		for _, layout := range []string{"01/02/2006", "1/2/06", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return formula.Serial(t), true
			}
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// Excel opens every file in Microsoft Excel for Mac, recalculates it and
// holds Excel's values to lfm's: the differences, or why Excel refused a
// file.
func (r Report) Excel(built []Built) ([]string, error) {
	files := map[string][]byte{}
	for _, f := range built {
		files[f.Name] = f.Data
	}
	opened, err := office.OpenInExcel(files)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range built {
		o := opened[f.Name]
		if o.Error != "" {
			out = append(out, f.Name+": "+o.Error)
			continue
		}
		// A file Excel did not save holds lfm's own values; comparing it
		// would prove nothing.
		if !bytes.Contains(appXML(o.Data), []byte("Microsoft")) {
			out = append(out, f.Name+": Excel did not save the recalculated file")
			continue
		}
		sheets, err := cellValues(o.Data)
		if err != nil {
			out = append(out, f.Name+": Excel's file cannot be read: "+err.Error())
			continue
		}
		for _, d := range compare(r.edition(f.Edition.Key), r.Books[f.Edition.Key][f.Dataset], sheets, "Excel") {
			out = append(out, f.Name+": "+d)
		}
	}
	return out, nil
}

// cellValues reads a workbook's raw cell values, by sheet name.
func cellValues(xlsx []byte) (map[string][][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][][]string{}
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, err
		}
		out[name] = rows
	}
	return out, nil
}

// appXML is a workbook's docProps/app.xml, which names the app that saved it.
func appXML(xlsx []byte) []byte {
	z, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		return nil
	}
	for _, f := range z.File {
		if f.Name == "docProps/app.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			return data
		}
	}
	return nil
}
