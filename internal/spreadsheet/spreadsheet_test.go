package spreadsheet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/ooxml"
	"github.com/Anonyfox/lfm/internal/project"
)

// fixture copies testdata/<name> into a fresh project's deliverables/spreadsheet.
func fixture(t *testing.T, name string, edit map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join("testdata", name))
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join("testdata", name, e.Name()))
		os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644)
	}
	for rel, content := range edit {
		os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644)
	}
	p, err := project.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func messages(ds []diagnostic.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Rule+" "+d.Location.File+":"+itoa(d.Location.Line)+": "+d.Message)
	}
	return strings.Join(out, "\n")
}

func itoa(n int) string { return strings.TrimSpace(strings.Repeat(" ", 0) + fmtInt(n)) }

func fmtInt(n int) string {
	s := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

var testConfig = Config{Path: "s7m2x9q4w8e3r6t5y1u0", Sheets: map[string]string{}}

func TestBudgetChecksAndComputes(t *testing.T) {
	r := Check(fixture(t, "budget", nil), testConfig, true, nil, "https://example.org/")
	var errs []diagnostic.Diagnostic
	for _, d := range r.Diagnostics {
		if d.Rule != RuleSheets {
			errs = append(errs, d)
		}
	}
	if len(errs) > 0 || !r.Buildable {
		t.Fatalf("findings:\n%s", messages(errs))
	}
	for _, ed := range r.Editions {
		b := r.Books[ed.Key][Example]
		for _, name := range []string{"income", "spent", "left", "goal", "safe", "days", "per_day", "needs", "wants", "saved", "budget", "over"} {
			v, _ := ed.lookup(b, name)
			t.Logf("%s %s = %s", ed.Key, name, v)
		}
	}
}

// LibreOffice computes every built file as lfm does; skipped without it.
func TestLibreOfficeAgrees(t *testing.T) {
	office, err := Office()
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	r := Check(fixture(t, "budget", nil), testConfig, true, nil, "https://example.org/")
	files := map[string][]byte{}
	for _, ed := range r.Editions {
		for _, ds := range []string{Empty, Example} {
			data, err := writeXLSX(ed, r.Books[ed.Key][ds], palette("#2F5D50"), "2026-10-19T00:00:00Z", "")
			if err != nil {
				t.Fatal(err)
			}
			files[ed.Key+"-"+ds+".xlsx"] = data
		}
	}
	got, err := Recalculate(office, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, ed := range r.Editions {
		for _, ds := range []string{Empty, Example} {
			if diff := compare(ed, r.Books[ed.Key][ds], got[ed.Key+"-"+ds+".xlsx"], "LibreOffice"); len(diff) > 0 {
				t.Errorf("%s %s:\n%s", ed.Key, ds, strings.Join(diff, "\n"))
			}
		}
	}
}

// Every function's edge cases, computed by lfm and by LibreOffice.
func TestEngineMatchesLibreOfficeOnEdgeCases(t *testing.T) {
	r := Check(fixture(t, "torture", nil), testConfig, true, nil, "https://example.org/")
	var errs []string
	for _, d := range r.Diagnostics {
		if d.Rule == RuleWorkbook {
			errs = append(errs, d.Message)
		}
	}
	if len(r.Editions) == 0 || len(errs) > 0 {
		t.Fatalf("%v", errs)
	}
	office, err := Office()
	if err != nil || testing.Short() {
		t.Skip(err)
	}
	ed := r.Editions[0]
	// Stamped, as every built file is: the zip comment must not disturb an app.
	data, err := writeXLSX(ed, r.Books["en"][Empty], palette(""), "2026-10-19T00:00:00Z", "0123456789abcdef")
	if err != nil || Stamp(data) != "0123456789abcdef" || Stamp(data[len(data)-300:]) != "0123456789abcdef" {
		t.Fatalf("%v, stamp %q", err, Stamp(data))
	}
	got, err := Recalculate(office, map[string][]byte{"t.xlsx": data})
	if err != nil {
		t.Fatal(err)
	}
	if diff := compare(ed, r.Books["en"][Empty], got["t.xlsx"], "LibreOffice"); len(diff) > 0 {
		t.Errorf("\n%s", strings.Join(diff, "\n"))
	}
}

func TestBuildReusesFilesOfTheSameInputs(t *testing.T) {
	r := Check(fixture(t, "budget", nil), testConfig, true, nil, "")
	first, err := r.Build(func(string) []byte { return nil })
	if err != nil || len(first) != 4 {
		t.Fatal(err, len(first))
	}
	kept := map[string][]byte{}
	for _, f := range first {
		kept[Stamp(f.Data)] = f.Data
	}
	if len(kept) != 4 || kept[""] != nil {
		t.Fatalf("every file needs its own stamp: %d", len(kept))
	}
	again, _ := r.Build(func(key string) []byte { return kept[key] })
	for i, f := range again {
		if &f.Data[0] != &first[i].Data[0] {
			t.Errorf("%s was written again", f.Name)
		}
	}
	changed := Check(fixture(t, "budget", map[string]string{Dir + "/concept.md": strings.Replace(read(t, "testdata/budget/concept.md"), "#2F5D50", "#1D4ED8", 1)}), testConfig, true, nil, "")
	if changed.Plan.Accent == r.Plan.Accent {
		t.Fatal("the fixture's accent did not change")
	}
	other, _ := changed.Build(func(key string) []byte { return kept[key] })
	for i, f := range other {
		if &f.Data[0] == &first[i].Data[0] {
			t.Errorf("%s was reused although its look changed", f.Name)
		}
	}
}

func TestBuildFailsOnWhatTheFileCannotHold(t *testing.T) {
	raw := read(t, "testdata/budget/workbook.json")
	wide := strings.Replace(raw, "\"widths\": [\n    2,", "\"widths\": [\n    300,", 1)
	if wide == raw {
		t.Fatal("the fixture's widths did not change")
	}
	r := Check(fixture(t, "budget", map[string]string{WorkbookFile: wide}), testConfig, true, nil, "")
	if _, err := r.Build(func(string) []byte { return nil }); err == nil || !strings.Contains(err.Error(), "column A") {
		t.Fatalf("a column too wide for Excel was left out silently: %v", err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUnportableFormulasAreReported(t *testing.T) {
	raw, _ := os.ReadFile("testdata/torture/workbook.json")
	broken := strings.Replace(string(raw), `"=SUM([data.n])"`, `"=\"x\"&(1>0)"`, 1)
	r := Check(fixture(t, "torture", map[string]string{WorkbookFile: broken}), testConfig, true, nil, "https://example.org/")
	if !strings.Contains(messages(r.Diagnostics), "joins TRUE or FALSE into text") {
		t.Fatal(messages(r.Diagnostics))
	}
}

// BenchmarkCheck is what every compile of a spreadsheet project pays to
// lay out, fill and compute every edition with every dataset.
func BenchmarkCheck(b *testing.B) {
	p := fixture(&testing.T{}, "budget", nil)
	for b.Loop() {
		Check(p, testConfig, true, nil, "")
	}
}

// TestExcelReadsEveryFile holds every built workbook to Microsoft's Open
// XML SDK, which reads them as Excel does. Set LFM_OOXML_VALIDATOR to the
// built validator to run it.
func TestExcelReadsEveryFile(t *testing.T) {
	dll := os.Getenv("LFM_OOXML_VALIDATOR")
	if dll == "" {
		t.Skip("LFM_OOXML_VALIDATOR is not set")
	}
	r := Check(fixture(t, "budget", nil), testConfig, true, nil, "")
	built, _ := r.Build(func(string) []byte { return nil })
	files := map[string][]byte{}
	for _, b := range built {
		files[b.Name] = b.Data
	}
	if out, err := ooxml.Validate(dll, files); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// TestExcelComputesAsLfm opens every built workbook in Microsoft Excel for
// Mac, recalculates it and holds every formula cell to lfm's value: the
// budget and the edge cases. LFM_EXCEL=1 runs it.
func TestExcelComputesAsLfm(t *testing.T) {
	if os.Getenv("LFM_EXCEL") == "" || !office.HasExcel() {
		t.Skip("set LFM_EXCEL=1 with Excel for Mac installed")
	}
	for _, name := range []string{"budget", "torture"} {
		r := Check(fixture(t, name, nil), testConfig, true, nil, "")
		built, err := r.Build(func(string) []byte { return nil })
		if err != nil {
			t.Fatal(err)
		}
		diffs, err := r.Excel(built)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range diffs {
			t.Errorf("%s: %s", name, d)
		}
	}
}
