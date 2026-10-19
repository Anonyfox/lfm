package compile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/scaffold"
	"github.com/Anonyfox/lfm/internal/spreadsheet"
)

const sheetPath = "x7m2x9q4w8e3r6t5y1u0"

func spreadsheetProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Create(dir); err != nil {
		t.Fatal(err)
	}
	configure(t, dir, func(c map[string]any) {
		c["deliverables"] = map[string]any{"spreadsheet": map[string]any{"path": sheetPath, "version": "2027.1",
			"sheets": map[string]any{"en": "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789/edit"}}}
		c["seller"] = map[string]any{"name": "Calm Money"}
	})
	src := filepath.Join("..", "spreadsheet", "testdata", "budget")
	dst := filepath.Join(dir, filepath.FromSlash(spreadsheet.Dir))
	os.MkdirAll(dst, 0o755)
	for _, name := range []string{"concept.md", "workbook.json"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dst, name), data, 0o644)
	}
	os.WriteFile(filepath.Join(dst, "listing.md"), []byte("# Listing: Paycheck Budget\n\n## Title\nPaycheck Budget Spreadsheet for Google Sheets and Excel\n\n## Description\n"+strings.Repeat("Know what is safe to spend until payday, every evening, in one simple list. ", 4)+"\n\n## Tags\n- budget spreadsheet\n\n## Price\n7.90 USD\n\n## AI\nText: assisted\nImages: none\nTools: Claude\nStatement: Texts edited with Claude; formulas computed and checked by software.\n"), 0o644)
	return dir
}

func TestSpreadsheetIsBuiltCheckedAndDelivered(t *testing.T) {
	t.Parallel()
	dir := spreadsheetProject(t)
	t.Cleanup(func() { strictArchives(t, dir) })
	result, err := Compile(context.Background(), dir, Options{Publish: true})
	if err != nil {
		t.Fatalf("%v\n%v", err, findings(result, ""))
	}
	var got []string
	for _, f := range findings(result, "spreadsheet.") {
		if !strings.Contains(f, "No Google Sheets master is linked") {
			got = append(got, f)
		}
	}
	if len(got) > 0 {
		t.Fatalf("findings: %v", got)
	}
	s := result.Spreadsheet
	if s == nil || s.Sheets != 6 || s.Formulas < 500 || s.Linked != 1 || len(s.Editions) != 2 {
		t.Fatalf("summary %+v", s)
	}
	out := filepath.Join(dir, "output", sheetPath)
	for _, name := range []string{"paycheck-budget-en.xlsx", "paycheck-budget-de-example.xlsx", "index.html"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !bytes.Contains(page, []byte("/copy")) {
		t.Error("the download page lacks the Google Sheets copy link")
	}
	if result.Previews[sheetPath+"/preview/de.html"] == nil {
		t.Error("no preview")
	}
	first, _ := os.ReadFile(filepath.Join(out, "paycheck-budget-en.xlsx"))
	again, _ := Compile(context.Background(), dir, Options{Publish: true})
	second, _ := os.ReadFile(filepath.Join(out, "paycheck-budget-en.xlsx"))
	if !bytes.Equal(first, second) || again.Spreadsheet == nil {
		t.Fatal("a rebuild of the unchanged workbook differs")
	}
	if _, err := release.BrowserPath(); err != nil || testing.Short() {
		return
	}
	result, err = Compile(context.Background(), dir, Options{Publish: true, Release: true})
	if err != nil || result.Spreadsheet.Pending || result.Spreadsheet.Images < 4 {
		t.Fatalf("release: %v %+v", err, result.Spreadsheet)
	}
	if _, err := spreadsheet.Office(); err == nil && !strings.HasPrefix(result.Spreadsheet.Compat, "LibreOffice agrees") {
		t.Fatalf("compat: %s %v", result.Spreadsheet.Compat, findings(result, "spreadsheet.compat"))
	}
	if _, err := os.Stat(filepath.Join(out, "paycheck-budget-de-start.pdf")); err != nil {
		t.Fatal("no start PDF")
	}
	guide, _ := os.ReadFile(filepath.Join(dir, "distribution", "spreadsheet", "stores", "etsy.md"))
	if !bytes.Contains(guide, []byte("paycheck-budget-en.zip")) || !bytes.Contains(guide, []byte("images/01-")) {
		t.Fatalf("etsy guide:\n%s", guide)
	}
	reused, _ := Compile(context.Background(), dir, Options{Publish: true})
	if reused.Spreadsheet.Pending || reused.Spreadsheet.Images != result.Spreadsheet.Images {
		t.Fatalf("prints not reused: %+v", reused.Spreadsheet)
	}
}
